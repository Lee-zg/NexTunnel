//go:build darwin

package p2p

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"unsafe"
)

// kernelTUN implements TUNDevice using macOS utun kernel interface.
type kernelTUN struct {
	file    *os.File
	name    string
	mtu     int
	localIP net.IP
	peerIP  net.IP
}

const (
	AF_SYSTEM       = 32
	AF_SYS_CONTROL  = 2
	SYS_CONTROL     = 2
	UTUN_OPT_IFNAME = 2
)

// CreateDarwinKernelTUNFile creates a real utun device and returns the backing
// file so a privileged helper can transfer it to the desktop process.
func CreateDarwinKernelTUNFile(cfg TUNConfig) (*os.File, string, int, error) {
	fd, err := syscall.Socket(AF_SYSTEM, syscall.SOCK_DGRAM, SYS_CONTROL)
	if err != nil {
		return nil, "", 0, fmt.Errorf("socket AF_SYSTEM: %w", err)
	}

	// 按 Darwin ctl_info 的真实布局查询 utun 控制器 ID。
	var ctlInfo struct {
		ctlID uint32
		name  [96]byte
	}
	copy(ctlInfo.name[:], "com.apple.net.utun_control")

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), 0xC0644E03, uintptr(unsafe.Pointer(&ctlInfo)))
	if errno != 0 {
		syscall.Close(fd)
		return nil, "", 0, fmt.Errorf("ioctl CTLIOCGINFO: %w", errno)
	}

	// sockaddr_ctl 必须包含 ss_sysaddr=AF_SYS_CONTROL，否则 connect 会失败或行为不确定。
	var sc struct {
		scLen     uint8
		scFamily  uint8
		ssSysAddr uint16
		scID      uint32
		scUnit    uint32
		reserved  [20]byte
	}
	sc.scLen = uint8(unsafe.Sizeof(sc))
	sc.scFamily = AF_SYSTEM
	sc.ssSysAddr = AF_SYS_CONTROL
	sc.scID = ctlInfo.ctlID
	sc.scUnit = 0 // auto-assign

	_, _, errno = syscall.Syscall(syscall.SYS_CONNECT, uintptr(fd), uintptr(unsafe.Pointer(&sc)), uintptr(unsafe.Sizeof(sc)))
	if errno != 0 {
		syscall.Close(fd)
		return nil, "", 0, fmt.Errorf("connect utun: %w", errno)
	}

	// Get device name
	var nameBuf [16]byte
	nameLen := uint32(len(nameBuf))
	_, _, errno = syscall.Syscall6(syscall.SYS_GETSOCKOPT, uintptr(fd), SYS_CONTROL, UTUN_OPT_IFNAME, uintptr(unsafe.Pointer(&nameBuf[0])), uintptr(unsafe.Pointer(&nameLen)), 0)
	if errno != 0 {
		syscall.Close(fd)
		return nil, "", 0, fmt.Errorf("getsockopt utun name: %w", errno)
	}
	devName := string(nameBuf[:nameLen-1])

	file := os.NewFile(uintptr(fd), "/dev/"+devName)
	mtu := cfg.MTU
	if mtu == 0 {
		mtu = 1420
	}
	return file, devName, mtu, nil
}

// NewDarwinKernelTUNFromFile wraps a utun file descriptor received from the
// privileged helper.
func NewDarwinKernelTUNFromFile(file *os.File, name string, cfg TUNConfig) TUNDevice {
	mtu := cfg.MTU
	if mtu == 0 {
		mtu = 1420
	}
	return &kernelTUN{
		file:    file,
		name:    name,
		mtu:     mtu,
		localIP: cfg.LocalIP,
		peerIP:  cfg.PeerIP,
	}
}

// createKernelTUN creates a real TUN device via utun on macOS.
func createKernelTUN(cfg TUNConfig) (TUNDevice, error) {
	file, name, _, err := CreateDarwinKernelTUNFile(cfg)
	if err != nil {
		return nil, err
	}
	return NewDarwinKernelTUNFromFile(file, name, cfg), nil
}

func (t *kernelTUN) ReadPacket(buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, fmt.Errorf("read packet buffer is empty")
	}
	// utun 是数据报 socket，4 字节地址族头和 IP 包必须一次性读取，不能分两次 Read。
	packet := make([]byte, len(buf)+4)
	n, err := t.file.Read(packet)
	if err != nil {
		return 0, err
	}
	if n < 4 {
		return 0, fmt.Errorf("utun packet too short: %d", n)
	}
	return copy(buf, packet[4:n]), nil
}

func (t *kernelTUN) WritePacket(buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, fmt.Errorf("write packet buffer is empty")
	}
	version := buf[0] >> 4
	var family uint32
	if version == 4 {
		family = syscall.AF_INET
	} else if version == 6 {
		family = syscall.AF_INET6
	} else {
		return 0, fmt.Errorf("unsupported IP version: %d", version)
	}
	packet := make([]byte, len(buf)+4)
	binary.BigEndian.PutUint32(packet[:4], family)
	copy(packet[4:], buf)
	n, err := t.file.Write(packet)
	if err != nil {
		return 0, err
	}
	if n != len(packet) {
		if n <= 4 {
			return 0, io.ErrShortWrite
		}
		return n - 4, io.ErrShortWrite
	}
	return len(buf), nil
}

func (t *kernelTUN) Close() error {
	return t.file.Close()
}

func (t *kernelTUN) MTU() (int, error) {
	return t.mtu, nil
}

func (t *kernelTUN) Name() (string, error) {
	return t.name, nil
}

func (t *kernelTUN) LocalAddr() net.IP {
	return t.localIP
}

func (t *kernelTUN) PeerAddr() net.IP {
	return t.peerIP
}
