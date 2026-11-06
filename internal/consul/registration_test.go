package consul

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func registrationOptions() RegistrationOptions {
	return RegistrationOptions{
		Hostname:   "node1",
		Address:    "10.0.0.7",
		Port:       8000,
		HealthPath: "/",
		Name:       "pdf-extractext-api",
		Version:    "1.2.3",
		Commit:     "abc123",
	}
}

func TestBuildRegistration_Snapshot(t *testing.T) {
	reg := BuildRegistration(registrationOptions())

	ruleValue := "Host(`pdf-extractext.localhost`)"
	ruleTag := "traefik.http.routers.api.rule=" + ruleValue
	expected := fmt.Sprintf(`{
		"ID":"api-node1",
		"Name":"pdf-extractext-api",
		"Address":"10.0.0.7",
		"Port":8000,
		"Tags":[
			"traefik.enable=true",
			"%s",
			"traefik.http.routers.api.entrypoints=https",
			"traefik.http.routers.api.tls=true",
			"traefik.http.services.api.loadbalancer.server.port=8000"
		],
		"Meta":{
			"traefik.enable":"true",
			"traefik.http.routers.api.rule":"%s",
			"traefik.http.routers.api.entrypoints":"https",
			"traefik.http.routers.api.tls":"true",
			"traefik.http.services.api.loadbalancer.server.port":"8000",
			"version":"1.2.3",
			"commit":"abc123"
		},
		"Check":{
			"HTTP":"http://10.0.0.7:8000/",
			"Interval":"10s",
			"Timeout":"2s",
			"DeregisterCriticalServiceAfter":"1m"
		}
	}`, ruleTag, ruleValue)

	assert.Equal(t, "api-node1", reg.ID)
	assert.JSONEq(t, expected, mustJSON(t, reg))
}

func TestBuildRegistration_UniqueIDs(t *testing.T) {
	first := BuildRegistration(RegistrationOptions{Hostname: "node-a", Address: "10.0.0.1", Port: 8000})
	second := BuildRegistration(RegistrationOptions{Hostname: "node-b", Address: "10.0.0.2", Port: 8000})

	assert.Equal(t, "api-node-a", first.ID)
	assert.Equal(t, "api-node-b", second.ID)
	assert.NotEqual(t, first.ID, second.ID)
}

func TestBuildRegistration_HealthCheckUsesAddressAndPort(t *testing.T) {
	reg := BuildRegistration(RegistrationOptions{
		Hostname:   "node1",
		Address:    "10.0.0.9",
		Port:       9000,
		HealthPath: "/healthz",
	})

	assert.Equal(t, "http://10.0.0.9:9000/healthz", reg.Check.HTTP)
	assert.Equal(t, "10s", reg.Check.Interval)
	assert.Equal(t, "2s", reg.Check.Timeout)
	assert.Equal(t, "1m", reg.Check.DeregisterCriticalServiceAfter)
}

func TestDetectLocalIP_UsesUDPRoute(t *testing.T) {
	var gotNetwork, gotAddress string
	dial := func(network, address string) (net.Conn, error) {
		gotNetwork = network
		gotAddress = address
		return &fakeConn{local: &net.UDPAddr{IP: net.ParseIP("10.0.0.7")}}, nil
	}

	ip, err := detectLocalIP("http://consul:8500", dial)
	require.NoError(t, err)
	assert.Equal(t, "udp", gotNetwork)
	assert.Equal(t, "consul:8500", gotAddress)
	assert.Equal(t, "10.0.0.7", ip)
}

func TestDetectLocalIP_RejectsLoopback(t *testing.T) {
	dial := func(network, address string) (net.Conn, error) {
		return &fakeConn{local: &net.UDPAddr{IP: net.ParseIP("127.0.0.1")}}, nil
	}

	_, err := detectLocalIP("consul:8500", dial)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loopback")
}

func TestDetectLocalIP_DialError(t *testing.T) {
	dial := func(network, address string) (net.Conn, error) {
		return nil, errors.New("sin ruta")
	}

	_, err := detectLocalIP("consul:8500", dial)
	require.Error(t, err)
}

func TestRegisterWithRetry_RetriesUntilSuccess(t *testing.T) {
	errBoom := errors.New("consul caído")
	registrar := &fakeRegistrar{registerErrs: []error{errBoom, errBoom, nil}}
	var sleeps []time.Duration

	err := registerWithRetry(t.Context(), registrar, BuildRegistration(registrationOptions()), time.Second, 4*time.Second, func(d time.Duration) {
		sleeps = append(sleeps, d)
	})

	require.NoError(t, err)
	assert.Equal(t, 3, registrar.registerCount())
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second}, sleeps)
}

func TestRegisterWithRetry_StopsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	registrar := &fakeRegistrar{registerErrs: []error{errors.New("consul caído"), errors.New("consul caído")}}

	err := registerWithRetry(ctx, registrar, BuildRegistration(registrationOptions()), time.Second, time.Second, func(time.Duration) {
		cancel()
	})

	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, registrar.registerCount())
}

func TestShutdown_DeregistersBeforeShutdown(t *testing.T) {
	registrar := &fakeRegistrar{}
	app := &fakeApp{}

	require.NoError(t, Shutdown(registrar, "api-node1", 0, app))

	assert.Equal(t, []string{"deregister:api-node1", "shutdown"}, registrar.eventsWith(app))
}

func TestShutdown_ContinuesWhenDeregisterFails(t *testing.T) {
	registrar := &fakeRegistrar{deregisterErr: errors.New("consul caído")}
	app := &fakeApp{}

	require.NoError(t, Shutdown(registrar, "api-node1", 0, app))

	assert.Equal(t, []string{"deregister:api-node1", "shutdown"}, registrar.eventsWith(app))
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}

type fakeRegistrar struct {
	mu            sync.Mutex
	registerErrs  []error
	deregisterErr error
	calls         int
	events        []string
}

func (f *fakeRegistrar) Register(ctx context.Context, payload any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.events = append(f.events, "register")
	if len(f.registerErrs) == 0 {
		return nil
	}
	err := f.registerErrs[0]
	f.registerErrs = f.registerErrs[1:]
	return err
}

func (f *fakeRegistrar) Deregister(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, "deregister:"+id)
	return f.deregisterErr
}

func (f *fakeRegistrar) registerCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeRegistrar) eventsWith(app *fakeApp) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append(append([]string{}, f.events...), app.events...)
}

type fakeApp struct {
	events []string
}

func (f *fakeApp) ShutdownWithTimeout(timeout time.Duration) error {
	f.events = append(f.events, "shutdown")
	return nil
}

type fakeConn struct {
	local net.Addr
}

func (c *fakeConn) Read(b []byte) (int, error)         { return 0, io.EOF }
func (c *fakeConn) Write(b []byte) (int, error)        { return len(b), nil }
func (c *fakeConn) Close() error                       { return nil }
func (c *fakeConn) LocalAddr() net.Addr                { return c.local }
func (c *fakeConn) RemoteAddr() net.Addr               { return c.local }
func (c *fakeConn) SetDeadline(t time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(t time.Time) error { return nil }
