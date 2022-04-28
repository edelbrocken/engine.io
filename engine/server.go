package engine

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/zishang520/engine.io/config"
	"github.com/zishang520/engine.io/errors"
	"github.com/zishang520/engine.io/events"
	"github.com/zishang520/engine.io/transports"
	"github.com/zishang520/engine.io/types"
	"github.com/zishang520/engine.io/utils"
	"io"
	"net/http"
	"strings"
	"time"
)

// Initialize websocket server
func (s *server) Init() {
	if !s.opts.Transports().Has("websocket") {
		return
	}
}

func (s *server) Cleanup() {
	utils.Log().Debug(`closing webSocketServer`)
	// s.ws.Close()
	// don't delete this.ws because it can be used again if the http server starts listening again
}

func (s *server) CreateTransport(transportName string, ctx *types.HttpContext) (transports.Transport, error) {
	if transport, ok := transports.Transports()[transportName]; ok {
		return transport.New(ctx), nil
	}
	return nil, errors.New("unsupported transportName").Err()
}

func (s *server) HandleRequest(ctx *types.HttpContext) {
	utils.Log().Debug(`handling "%s" http request "%s"`, ctx.Method(), ctx.Request().RequestURI)

	callback := func(errorCode int, errorContext map[string]interface{}) {
		if errorContext != nil {
			s.Emit("connection_error", &types.ErrorMessage{
				CodeMessage: &types.CodeMessage{
					Code:    errorCode,
					Message: errorMessages[errorCode],
				},
				Req:     ctx,
				Context: errorContext,
			})
			abortRequest(ctx, errorCode, errorContext)
			return
		}

		if sid := ctx.Query().Peek("sid"); sid != "" {
			utils.Log().Debug("setting new request for existing client")
			if socket, ok := s.clients.Load(sid); ok {
				socket.(Socket).Transport().OnRequest(ctx)
			} else {
				abortRequest(ctx, UNKNOWN_SID, map[string]interface{}{"sid": sid})
			}
		} else {
			if errorCode, errorContext, t := s.Handshake(ctx.Query().Peek("transport"), ctx); t == nil {
				abortRequest(ctx, errorCode, errorContext)
			}
		}
	}

	if s.corsMiddleware != nil {
		s.corsMiddleware(ctx, func() {
			callback(s.Verify(ctx, false))
		})
	} else {
		callback(s.Verify(ctx, false))
	}
}

func (s *server) HandleUpgrade(ctx *types.HttpContext) {
	errorCode, errorContext := s.Verify(ctx, true)
	if errorContext != nil {
		s.Emit("connection_error", &types.ErrorMessage{
			CodeMessage: &types.CodeMessage{
				Code:    errorCode,
				Message: errorMessages[errorCode],
			},
			Req:     ctx,
			Context: errorContext,
		})
		abortUpgrade(ctx, errorCode, errorContext)
		return
	}

	wsc := &types.WebSocketConn{EventEmitter: events.New()}

	ws := &websocket.Upgrader{
		ReadBufferSize:    1024,
		WriteBufferSize:   1024,
		EnableCompression: s.opts.PerMessageDeflate() != nil,
		Error: func(_ http.ResponseWriter, _ *http.Request, _ int, reason error) {
			if websocket.IsUnexpectedCloseError(reason) {
				wsc.Emit("close")
			} else {
				wsc.Emit("error", reason)
			}
		},
		CheckOrigin: func(*http.Request) bool {
			if allowRequest := s.opts.AllowRequest(); allowRequest != nil {
				if code, err := allowRequest(ctx); code != OK_REQUEST || err != nil {
					return false
				}
			}
			return true
		},
	}

	// delegate to ws
	if conn, err := ws.Upgrade(ctx.Response(), ctx.Request(), ctx.Response().Header()); err == nil {
		conn.SetReadLimit(s.opts.MaxHttpBufferSize())
		wsc.Conn = conn
		s.onWebSocket(ctx, wsc)
	} else {
		utils.Log().Debug("websocket error before upgrade: %s", err)
	}

}

func (s *server) onWebSocket(ctx *types.HttpContext, wsc *types.WebSocketConn) {
	onUpgradeError := func(...interface{}) {
		utils.Log().Debug("websocket error before upgrade")
		// wsc.close() not needed
	}

	wsc.On("error", onUpgradeError)

	transportName := ctx.Query().Peek("transport")
	if transport, ok := transports.Transports()[transportName]; ok && !transport.HandlesUpgrades {
		utils.Log().Debug("transport doesnt handle upgraded requests")
		wsc.Close()
		return
	}

	// get client id
	id := ctx.Query().Peek("sid")

	// keep a reference to the ws.Socket
	ctx.Websocket = wsc

	if len(id) > 0 {
		client, ok := s.clients.Load(id)

		if !ok {
			utils.Log().Debug("upgrade attempt for closed client")
			wsc.Close()
		} else if client.(Socket).Upgrading() {
			utils.Log().Debug("transport has already been trying to upgrade")
			wsc.Close()
		} else if client.(Socket).Upgraded() {
			utils.Log().Debug("transport had already been upgraded")
			wsc.Close()
		} else {
			utils.Log().Debug("upgrading existing transport")

			// transport error handling takes over
			wsc.RemoveListener("error", onUpgradeError)

			transport, err := s.CreateTransport(transportName, ctx)
			if err != nil {
				utils.Log().Debug("upgrading not existing transport")
				wsc.Close()
			} else {
				if ctx.Query().Has("b64") {
					transport.SetSupportsBinary(false)
				} else {
					transport.SetSupportsBinary(true)
				}
				transport.SetPerMessageDeflate(s.opts.PerMessageDeflate())
				client.(Socket).MaybeUpgrade(transport)
			}
		}
	} else {
		if errorCode, errorContext, t := s.Handshake(transportName, ctx); t == nil {
			abortUpgrade(ctx, errorCode, errorContext)
		}
	}
}

func (s *server) Attach(server *types.HttpServer, opts interface{}) {
	options, _ := opts.(*config.AttachOptions)
	path := "/engine.io"
	destroyUpgradeTimeout := 1000 * time.Millisecond

	if options != nil {
		if options.Path != nil {
			path = strings.TrimRight(options.Path(), "/")
		}

		if options.DestroyUpgradeTimeout != nil {
			destroyUpgradeTimeout = options.DestroyUpgradeTimeout()
		}
	}

	// normalize path
	path += "/"

	server.RegisterOnShutdown(func() {
		s.Close()
	})
	server.On("listening", func(...interface{}) {
		s.Init()
	})
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := types.NewHttpContext(w, r)
		if !websocket.IsWebSocketUpgrade(r) {
			if strings.HasPrefix(utils.CleanPath(r.URL.Path), path) {
				utils.Log().Debug(`intercepting request for path "%s"`, path)
				s.HandleRequest(ctx)
				<-ctx.Done()
			} else {
				server.DefaultHandler.ServeHTTP(ctx.Response(), ctx.Request())
			}
		} else if s.opts.Transports().Has("websocket") {
			if strings.HasPrefix(utils.CleanPath(r.URL.Path), path) {
				s.HandleUpgrade(ctx)
			} else if options.DestroyUpgrade() {
				// default node behavior is to disconnect when no handlers
				// but by adding a handler, we prevent that
				// and if no eio thing handles the upgrade
				// then the socket needs to die!
				utils.SetTimeOut(func() {
					if !ctx.IsDone() {
						ctx.Write(nil)
					}
				}, destroyUpgradeTimeout)
				<-ctx.Done()
			}
		} else {
			server.DefaultHandler.ServeHTTP(ctx.Response(), ctx.Request())
		}
	})
}

func abortRequest(ctx *types.HttpContext, errorCode int, errorContext map[string]interface{}) {
	utils.Log().Debug("abortRequest %d", errorCode)
	statusCode := http.StatusBadRequest
	if errorCode == FORBIDDEN {
		statusCode = http.StatusForbidden
	}
	message := errorMessages[errorCode]
	if m, ok := errorContext["message"]; ok {
		message = m.(string)
	}
	ctx.Response().Header().Set("Content-Type", "application/json")
	ctx.SetStatusCode(statusCode)
	if b, err := json.Marshal(types.CodeMessage{Code: errorCode, Message: message}); err == nil {
		ctx.Write(b)
	} else {
		io.WriteString(ctx, `{"code":400,"message":"Bad request"}`)
	}
}

func abortUpgrade(ctx *types.HttpContext, errorCode int, errorContext map[string]interface{}) {
	utils.Log().Debug("abortUpgrade %d", errorCode)
	message := errorMessages[errorCode]
	if m, ok := errorContext["message"]; ok {
		message = m.(string)
	}
	ctx.SetStatusCode(http.StatusBadRequest)
	io.WriteString(ctx, message)
}
