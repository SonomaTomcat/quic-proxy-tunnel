// quic.go
// Provides QUIC network listener, dialer, and stream helpers for the proxy.
//
// Contains QuicListener, QuicDialer, QuicStream, and related logic for QUIC-based proxying.

package common

import (
	"context"
	"crypto/tls"
	log "github.com/SonomaTomcat/quic-proxy-tunnel/util"
	"net"
	"sync"

	quic "github.com/quic-go/quic-go"
)

const (
	KQuicProxy = "quic-proxy"
)

// QuicListener wraps a quic.Listener and provides Accept/Addr/Close for net.Listener compatibility.
type QuicListener struct {
	Listener     *quic.Listener
	chAcceptConn chan *AcceptConn
}

// AcceptConn holds a net.Conn and error for async accept.
type AcceptConn struct {
	conn net.Conn
	err  error
}

// NewQuicListener creates a QuicListener from a quic.Listener.
func NewQuicListener(l *quic.Listener) *QuicListener {
	ql := &QuicListener{
		Listener:     l,
		chAcceptConn: make(chan *AcceptConn, 4),
	}
	go ql.doAccept()
	return ql
}

// doAccept continuously accepts incoming QUIC connections and streams, and forwards them to the accept channel.
func (ql *QuicListener) doAccept() {
	log.Info("QuicListener waiting for connection on %s", ql.Listener.Addr())
	for {
		conn, err := ql.Listener.Accept(context.TODO())
		if err != nil {
			log.Error("accept connection failed:%v", err)
			continue
		}
		log.Info("QuicListener accepted new connection from %s", conn.RemoteAddr())

		go func(conn quic.Connection) {
			for {
				stream, err := conn.AcceptStream(context.TODO())
				if err != nil {
					log.Notice("accept stream failed:%v", err)
					err := conn.CloseWithError(2020, err.Error())
					if err != nil {
						return
					}
					return
				}
				log.Info("QuicListener accepted stream %v from %s", stream.StreamID(), conn.RemoteAddr())
				ql.chAcceptConn <- &AcceptConn{
					conn: &QuicStream{conn: conn, Stream: stream},
					err:  nil,
				}
			}
		}(conn)
	}
}

// Accept waits for and returns the next connection to the listener.
func (ql *QuicListener) Accept() (net.Conn, error) {
	ac := <-ql.chAcceptConn
	return ac.conn, ac.err
}

// Addr returns the local network address of the listener.
func (ql *QuicListener) Addr() net.Addr {
	return ql.Listener.Addr()
}

// Close closes the listener.
func (ql *QuicListener) Close() error {
	return ql.Listener.Close()
}

// QuicStream wraps a quic.Stream and its parent connection.
type QuicStream struct {
	conn quic.Connection
	quic.Stream
}

// LocalAddr returns the local network address of the stream.
func (qs *QuicStream) LocalAddr() net.Addr {
	return qs.conn.LocalAddr()
}

// RemoteAddr returns the remote network address of the stream.
func (qs *QuicStream) RemoteAddr() net.Addr {
	return qs.conn.RemoteAddr()
}

// QuicDialer manages a persistent QUIC connection for dialing streams.
type QuicDialer struct {
	skipCertVerify bool
	conn           quic.Connection
	sync.Mutex
}

// NewQuicDialer returns a new QuicDialer.
func NewQuicDialer(skipCertVerify bool) *QuicDialer {
	return &QuicDialer{
		skipCertVerify: skipCertVerify,
	}
}

// Dial opens a new stream to the QUIC server, and sends the target address as the first line.
func (qd *QuicDialer) Dial(serverAddr, targetAddr string) (net.Conn, error) {
	qd.Lock()
	defer qd.Unlock()

	log.Info("QuicDialer dialing server %s for target %s", serverAddr, targetAddr)

	if qd.conn == nil {
		conn, err := quic.DialAddr(
			context.TODO(),
			serverAddr,
			&tls.Config{
				InsecureSkipVerify: qd.skipCertVerify,
				NextProtos:         []string{KQuicProxy},
			},
			nil,
		)
		if err != nil {
			log.Error("dial connection failed:%v", err)
			return nil, err
		}
		log.Info("QuicDialer established connection to %s", serverAddr)
		qd.conn = conn
	}

	stream, err := qd.conn.OpenStreamSync(context.TODO())
	if err != nil {
		log.Info("[1/2] open stream from connection no success:%v, try to open new connection", err)
		err := qd.conn.CloseWithError(2021, err.Error())
		if err != nil {
			return nil, err
		}
		conn, err := quic.DialAddr(
			context.TODO(),
			serverAddr,
			&tls.Config{
				InsecureSkipVerify: true,
				NextProtos:         []string{KQuicProxy},
			},
			nil,
		)
		if err != nil {
			log.Error("[2/2] dial new connection failed:%v", err)
			return nil, err
		}
		log.Info("QuicDialer re-established connection to %s", serverAddr)
		qd.conn = conn

		stream, err = qd.conn.OpenStreamSync(context.TODO())
		if err != nil {
			log.Error("[2/2] open stream from new connection failed:%v", err)
			return nil, err
		}
		log.Info("[2/2] open stream from new connection OK")
	}

	// Send the target address to the server, protocol: target address + newline
	_, err = stream.Write([]byte(targetAddr + "\n"))
	if err != nil {
		log.Error("failed to send target address to server: %v", err)
		stream.Close()
		return nil, err
	}

	log.Info("QuicDialer opened stream to %s, stream_id:%v, target:%s", serverAddr, stream.StreamID(), targetAddr)
	return &QuicStream{conn: qd.conn, Stream: stream}, nil
}

// ListenAddr creates a QUIC listener.
func ListenAddr(addr string, tlsConf *tls.Config) (*quic.Listener, error) {
	return quic.ListenAddr(addr, tlsConf, nil)
}
