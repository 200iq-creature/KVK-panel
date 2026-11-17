package app

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"sync"
	"syscall"
	"time"

	awgtun "github.com/amnezia-vpn/amneziawg-go/v3/tun"
	"github.com/sagernet/gvisor/pkg/buffer"
	"github.com/sagernet/gvisor/pkg/tcpip"
	"github.com/sagernet/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/sagernet/gvisor/pkg/tcpip/header"
	"github.com/sagernet/gvisor/pkg/tcpip/link/channel"
	"github.com/sagernet/gvisor/pkg/tcpip/network/ipv4"
	"github.com/sagernet/gvisor/pkg/tcpip/network/ipv6"
	"github.com/sagernet/gvisor/pkg/tcpip/stack"
	"github.com/sagernet/gvisor/pkg/tcpip/transport/tcp"
	"github.com/sagernet/gvisor/pkg/tcpip/transport/udp"
	"github.com/sagernet/gvisor/pkg/waiter"
)

// gvStack is a userspace TCP/IP stack exposed to amneziawg-go as a TUN
// device. Every TCP connection and UDP flow coming out of the tunnel is
// terminated here and forwarded through the xray bridge — no kernel TUN,
// routing or NAT is needed on the server.

type flowDialer func(src netip.Addr, dst bridgeDest) (*bridgeConn, error)

type gvStack struct {
	ep       *channel.Endpoint
	s        *stack.Stack
	mtu      int
	events   chan awgtun.Event
	incoming chan *buffer.View
	notify   *channel.NotificationHandle
	once     sync.Once
	ctx      context.Context
	cancel   context.CancelFunc
}

func newGVStack(mtu int, onTCP, onUDP flowDialer) (*gvStack, error) {
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
		HandleLocal:        false,
	})
	g := &gvStack{
		ep:       channel.New(1024, uint32(mtu), ""),
		s:        s,
		mtu:      mtu,
		events:   make(chan awgtun.Event, 4),
		incoming: make(chan *buffer.View, 1024),
	}
	g.ctx, g.cancel = context.WithCancel(context.Background())
	sack := tcpip.TCPSACKEnabled(true)
	s.SetTransportProtocolOption(tcp.ProtocolNumber, &sack)
	g.notify = g.ep.AddNotify(g)
	if err := s.CreateNIC(1, g.ep); err != nil {
		return nil, errors.New(err.String())
	}
	// accept traffic for any destination and answer from any source address
	s.SetPromiscuousMode(1, true)
	s.SetSpoofing(1, true)
	s.SetRouteTable([]tcpip.Route{
		{Destination: header.IPv4EmptySubnet, NIC: 1},
		{Destination: header.IPv6EmptySubnet, NIC: 1},
	})

	tcpFwd := tcp.NewForwarder(s, 0, 4096, func(r *tcp.ForwarderRequest) {
		id := r.ID()
		src := addrFrom(id.RemoteAddress)
		dst := bridgeDest{IP: addrFrom(id.LocalAddress), Port: id.LocalPort}
		bc, err := onTCP(src, dst)
		if err != nil {
			r.Complete(true)
			return
		}
		var wq waiter.Queue
		ep, terr := r.CreateEndpoint(&wq)
		if terr != nil {
			bc.Close()
			r.Complete(true)
			return
		}
		r.Complete(false)
		ep.SocketOptions().SetKeepAlive(true)
		pipe(gonet.NewTCPConn(&wq, ep), bc)
	})
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, tcpFwd.HandlePacket)

	udpFwd := udp.NewForwarder(s, func(r *udp.ForwarderRequest) bool {
		id := r.ID()
		src := addrFrom(id.RemoteAddress)
		dst := bridgeDest{IP: addrFrom(id.LocalAddress), Port: id.LocalPort}
		var wq waiter.Queue
		ep, terr := r.CreateEndpoint(&wq)
		if terr != nil {
			return true
		}
		uc := gonet.NewUDPConn(&wq, ep)
		go relayUDP(uc, src, dst, onUDP)
		return true
	})
	s.SetTransportProtocolHandler(udp.ProtocolNumber, udpFwd.HandlePacket)

	g.events <- awgtun.EventUp
	return g, nil
}

func addrFrom(a tcpip.Address) netip.Addr {
	ip, _ := netip.AddrFromSlice(a.AsSlice())
	return ip.Unmap()
}

// relayUDP forwards one UDP flow (client src -> dst) through the bridge.
func relayUDP(uc *gonet.UDPConn, src netip.Addr, dst bridgeDest, dial flowDialer) {
	defer uc.Close()
	bc, err := dial(src, dst)
	if err != nil {
		return
	}
	defer bc.Close()
	const idle = 2 * time.Minute
	go func() {
		p := make([]byte, 65535)
		for {
			_ = bc.SetReadDeadline(time.Now().Add(idle))
			n, err := bc.ReadPacket(p)
			if err != nil {
				uc.Close()
				return
			}
			if n > 0 {
				if _, err := uc.Write(p[:n]); err != nil {
					return
				}
			}
		}
	}()
	p := make([]byte, 65535)
	for {
		_ = uc.SetReadDeadline(time.Now().Add(idle))
		n, err := uc.Read(p)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return
			}
			return
		}
		if err := bc.WritePacket(p[:n]); err != nil {
			return
		}
	}
}

// WriteNotify is called by the channel endpoint when the stack emits a packet.
func (g *gvStack) WriteNotify() {
	pkt := g.ep.Read()
	if pkt == nil {
		return
	}
	v := pkt.ToView()
	pkt.DecRef()
	select {
	case g.incoming <- v:
	case <-g.ctx.Done():
		v.Release()
	}
}

// ---- awgtun.Device ----

func (g *gvStack) File() *os.File              { return nil }
func (g *gvStack) Name() (string, error)       { return "kvk-awg", nil }
func (g *gvStack) MTU() (int, error)           { return g.mtu, nil }
func (g *gvStack) Events() <-chan awgtun.Event { return g.events }
func (g *gvStack) BatchSize() int              { return 1 }

func (g *gvStack) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	select {
	case v := <-g.incoming:
		n, err := v.Read(bufs[0][offset:])
		v.Release()
		if err != nil {
			return 0, err
		}
		sizes[0] = n
		return 1, nil
	case <-g.ctx.Done():
		return 0, os.ErrClosed
	}
}

func (g *gvStack) Write(bufs [][]byte, offset int) (int, error) {
	for _, b := range bufs {
		pkt := b[offset:]
		if len(pkt) == 0 {
			continue
		}
		pb := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(pkt)})
		switch pkt[0] >> 4 {
		case 4:
			g.ep.InjectInbound(header.IPv4ProtocolNumber, pb)
		case 6:
			g.ep.InjectInbound(header.IPv6ProtocolNumber, pb)
		default:
			pb.DecRef()
			return 0, syscall.EAFNOSUPPORT
		}
		pb.DecRef()
	}
	return len(bufs), nil
}

func (g *gvStack) Close() error {
	g.once.Do(func() {
		g.cancel()
		g.ep.RemoveNotify(g.notify)
		g.s.RemoveNIC(1)
		g.s.Close()
		g.ep.Close()
		close(g.events)
	})
	return nil
}
