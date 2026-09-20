package smtp

import (
	"context"
	"errors"
	"net"
	"os"
	"time"

	"github.com/warmbly/warmbly/internal/client/netbind"
	"github.com/warmbly/warmbly/internal/models"
)

// The two submission ports: 465 speaks TLS from the first byte, 587 upgrades
// with STARTTLS. Every major provider listens on both.
const (
	PortSMTPS      = 465
	PortSubmission = 587
)

// fallbackHeadStart is how long 465 has to itself before 587 is dialled
// alongside it. A reachable 465 connects well inside it, so a healthy network
// never sees the second dial; a blocked one costs this much, not a dial timeout.
const fallbackHeadStart = time.Second

// dialOutcome is what one dial of the race came back with.
type dialOutcome struct {
	conn net.Conn
	err  error
	port int
}

// Dialed is an open submission socket and the mode to speak on it. Port and
// Security are the mailbox's own unless FellBack is set.
type Dialed struct {
	Conn     net.Conn
	Port     int
	Security string
	FellBack bool
}

// dialTCP opens one TCP connection; a variable so the race can be tested
// without a network that swallows SYNs.
var dialTCP = func(ctx context.Context, local *net.TCPAddr, addr string) (net.Conn, error) {
	return netbind.Dialer(local).DialContext(ctx, "tcp", addr)
}

// DialSubmission opens the socket for a mailbox's SMTP server (host already
// normalized). A mailbox on 465 whose SYN goes unanswered is dialled on 587
// as well, in parallel after a head start, and whichever connects first is
// used: a port that stays silent is a network in the way, not the server,
// and hosts that block outbound 465 mostly leave 587 open. A refused port or
// a name that does not resolve comes back at once and is not retried. When
// nothing connects the error is 465's own.
func DialSubmission(ctx context.Context, local *net.TCPAddr, host string, port int, security string) (Dialed, error) {
	resolved := models.ResolveSMTPSecurity(security, port)
	if resolved != models.MailSecurityTLS || port != PortSMTPS {
		conn, err := dialTCP(ctx, local, models.MailDialAddress(host, port))
		return Dialed{Conn: conn, Port: port, Security: resolved}, err
	}

	raceCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan dialOutcome, 2)
	dial := func(port int) {
		conn, err := dialTCP(raceCtx, local, models.MailDialAddress(host, port))
		results <- dialOutcome{conn: conn, err: err, port: port}
	}
	inFlight := 1
	go dial(PortSMTPS)

	headStart := time.NewTimer(fallbackHeadStart)
	defer headStart.Stop()
	fallbackStarted := false
	startFallback := func() {
		if fallbackStarted {
			return
		}
		fallbackStarted = true
		inFlight++
		go dial(PortSubmission)
	}

	var primaryErr error
	for inFlight > 0 {
		select {
		case <-headStart.C:
			startFallback()
		case r := <-results:
			inFlight--
			if r.err == nil && r.conn != nil {
				go closeLosers(results, inFlight)
				return Dialed{Conn: r.conn, Port: r.port, Security: securityForPort(r.port), FellBack: r.port != port}, nil
			}
			if r.port != PortSMTPS {
				continue
			}
			primaryErr = r.err
			if primaryErr == nil {
				primaryErr = errors.New("dial returned no connection")
			}
			if dialTimedOut(r.err) {
				startFallback()
			} else if !fallbackStarted {
				return Dialed{}, primaryErr
			}
		}
	}
	return Dialed{}, primaryErr
}

// closeLosers drains the dials still in flight after a winner was taken, so
// a socket that connects late is closed rather than leaked.
func closeLosers(results <-chan dialOutcome, n int) {
	for ; n > 0; n-- {
		if r := <-results; r.conn != nil {
			r.conn.Close()
		}
	}
}

func securityForPort(port int) string {
	if port == PortSMTPS {
		return models.MailSecurityTLS
	}
	return models.MailSecurityStartTLS
}

// dialTimedOut reports whether a dial ended on a deadline rather than on an
// answer from the network.
func dialTimedOut(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded)
}
