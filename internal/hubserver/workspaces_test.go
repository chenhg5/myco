package hubserver

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestWorkspaceShutdownWaitsForWorkers(t *testing.T) {
	for _, wholeServer := range []bool{false, true} {
		name := "workspace"
		if wholeServer {
			name = "server"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				release := make(chan struct{})
				closed := make(chan struct{})
				done := make(chan struct{})
				w := &workspaceHub{stop: cancel, hub: &Hub{Close: func() error {
					close(closed)
					return nil
				}}}
				// Model a worker finishing a query or closing a file after
				// cancellation. Shutdown must let it release its resources
				// before closing the database and archiving the directory.
				w.run(ctx, func(ctx context.Context) {
					<-ctx.Done()
					<-release
				})
				go func() {
					defer close(done)
					if wholeServer {
						ws := &workspaces{hubs: map[string]*workspaceHub{"demo": w}}
						ws.close()
					} else if err := w.close(); err != nil {
						t.Error(err)
					}
				}()
				synctest.Wait()
				if ctx.Err() == nil {
					t.Fatal("shutdown did not cancel the worker")
				}
				select {
				case <-closed:
					t.Error("database closed before the worker released its resources")
				default:
				}
				close(release)
				synctest.Wait()
				select {
				case <-done:
				default:
					t.Fatal("shutdown did not finish after the worker returned")
				}
				select {
				case <-closed:
				default:
					t.Fatal("shutdown did not close the database")
				}
			})
		})
	}
}
