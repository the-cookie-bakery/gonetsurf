package shells

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"gonetsurf/brokers/network"
	"gonetsurf/brokers/storage"
	"gonetsurf/ipc"
	"gonetsurf/urls"
)

func TestShellUsesBrokers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		writer.Write([]byte(`<!doctype html><html><head><title>Broker Page</title></head><body><h1>Broker Page</h1></body></html>`))
	}))
	defer server.Close()
	target, err := urls.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	token, _ := ipc.NewCapability()
	networkServerSide, networkClientSide := net.Pipe()
	defer networkServerSide.Close()
	defer networkClientSide.Close()
	networkBroker := &network.Server{
		Conn:    ipc.NewConn(networkServerSide),
		Token:   token,
		Fetcher: network.HTTPFetcher{},
	}
	go networkBroker.Serve(context.Background())
	storageServerSide, storageClientSide := net.Pipe()
	defer storageServerSide.Close()
	defer storageClientSide.Close()
	storageBroker := &storage.Server{
		Conn:  ipc.NewConn(storageServerSide),
		Token: token,
		Store: storage.NewStore(),
	}
	go storageBroker.Serve(context.Background())

	shell := New(400, 300, nil, BrokerClients{
		Network: &network.Client{Conn: ipc.NewConn(networkClientSide), Token: token},
		Storage: &storage.Client{Conn: ipc.NewConn(storageClientSide), Token: token},
	})
	shell.NavigateInitial(target)
	if shell.page == nil {
		t.Fatalf("page did not load: %q", shell.Error)
	}
	if shell.Browser.ActiveTab().Title != "Broker Page" {
		t.Errorf("title = %q", shell.Browser.ActiveTab().Title)
	}
	if rendered, renderErr := shell.Render(); renderErr == nil {
		_ = rendered.Image()
	}
	if err := shell.SaveSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := shell.LoadSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	if shell.Browser.ActiveTab() == nil || shell.Browser.ActiveTab().URL == nil {
		t.Error("session restore lost the active tab")
	}
}
