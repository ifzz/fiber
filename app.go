// ⚡️ Fiber is an Express inspired web framework written in Go with ☕️
// 🤖 Github Repository: https://github.com/gofiber/fiber
// 📌 API Documentation: https://docs.gofiber.io

package fiber

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	utils "github.com/gofiber/utils"
	colorable "github.com/segrey/go-colorable"
	fasthttp "github.com/valyala/fasthttp"
)

// Version of current package
const Version = "1.12.1"

// Map is a shortcut for map[string]interface{}, useful for JSON returns
type Map map[string]interface{}

// Handler defines a function to serve HTTP requests.
type Handler = func(*Ctx)

// Error represents an error that occurred while handling a request.
type Error struct {
	Code    int
	Message string
}

// App denotes the Fiber application.
type App struct {
	mutex sync.Mutex
	// Route stack divided by HTTP methods
	stack [][]*Route
	// Amount of registered routes
	routes int
	// Ctx pool
	pool sync.Pool
	// Fasthttp server
	server *fasthttp.Server
	// App settings
	Settings *Settings
}

// Settings holds is a struct holding the server settings
type Settings struct {
	// ErrorHandler is executed when you pass an error in the Next(err) method
	// This function is also executed when middleware.Recover() catches a panic
	// Default: func(ctx *Ctx, err error) {
	// 	code := StatusInternalServerError
	// 	if e, ok := err.(*Error); ok {
	// 		code = e.Code
	// 	}
	// 	ctx.Set(HeaderContentType, MIMETextPlainCharsetUTF8)
	// 	ctx.Status(code).SendString(err.Error())
	// }
	ErrorHandler func(*Ctx, error)

	// Enables the "Server: value" HTTP header.
	// Default: ""
	ServerHeader string

	// Enable strict routing. When enabled, the router treats "/foo" and "/foo/" as different.
	// By default this is disabled and both "/foo" and "/foo/" will execute the same handler.
	StrictRouting bool

	// Enable case sensitive routing. When enabled, "/FoO" and "/foo" are different routes.
	// By default this is disabled and both "/FoO" and "/foo" will execute the same handler.
	CaseSensitive bool

	// Enables handler values to be immutable even if you return from handler
	// Default: false
	Immutable bool

	// Enable or disable ETag header generation, since both weak and strong etags are generated
	// using the same hashing method (CRC-32). Weak ETags are the default when enabled.
	// Default value false
	ETag bool

	// This will spawn multiple Go processes listening on the same port
	// Default: false
	Prefork bool

	// Max body size that the server accepts
	// Default: 4 * 1024 * 1024
	BodyLimit int

	// Maximum number of concurrent connections.
	// Default: 256 * 1024
	Concurrency int

	// Disable keep-alive connections, the server will close incoming connections after sending the first response to client
	// Default: false
	DisableKeepalive bool

	// When set to true causes the default date header to be excluded from the response.
	// Default: false
	DisableDefaultDate bool

	// When set to true, causes the default Content-Type header to be excluded from the Response.
	// Default: false
	DisableDefaultContentType bool

	// By default all header names are normalized: conteNT-tYPE -> Content-Type
	// Default: false
	DisableHeaderNormalizing bool

	// When set to true, it will not print out the «Fiber» ASCII art and listening address
	// Default: false
	DisableStartupMessage bool

	// Templates is deprecated please use Views
	// Default: nil
	Templates Templates

	// Views is the interface that wraps the Render function.
	// Default: nil
	Views Views

	// The amount of time allowed to read the full request including body.
	// It is reset after the request handler has returned.
	// The connection's read deadline is reset when the connection opens.
	// Default: unlimited
	ReadTimeout time.Duration

	// The maximum duration before timing out writes of the response.
	// It is reset after the request handler has returned.
	// Default: unlimited
	WriteTimeout time.Duration

	// The maximum amount of time to wait for the next request when keep-alive is enabled.
	// If IdleTimeout is zero, the value of ReadTimeout is used.
	// Default: unlimited
	IdleTimeout time.Duration

	// Per-connection buffer size for requests' reading.
	// This also limits the maximum header size.
	// Increase this buffer if your clients send multi-KB RequestURIs
	// and/or multi-KB headers (for example, BIG cookies).
	// Default 4096
	ReadBufferSize int

	// Per-connection buffer size for responses' writing.
	// Default 4096
	WriteBufferSize int

	// CompressedFileSuffix adds suffix to the original file name and
	// tries saving the resulting compressed file under the new file name.
	// Default: ".fiber.gz"
	CompressedFileSuffix string

	// FEATURE: v1.13
	// The router executes the same handler by default if StrictRouting or CaseSensitive is disabled.
	// Enabling RedirectFixedPath will change this behaviour into a client redirect to the original route path.
	// Using the status code 301 for GET requests and 308 for all other request methods.
	// RedirectFixedPath bool
}

// Static struct
type Static struct {
	// This works differently than the github.com/gofiber/compression middleware
	// The server tries minimizing CPU usage by caching compressed files.
	// Optional. Default value false
	Compress bool

	// Enables byte range requests if set to true.
	// Optional. Default value false
	ByteRange bool

	// Enable directory browsing.
	// Optional. Default value false.
	Browse bool

	// Index file for serving a directory.
	// Optional. Default value "index.html".
	Index string
}

// default settings
var (
	defaultBodyLimit       = 4 * 1024 * 1024
	defaultConcurrency     = 256 * 1024
	defaultReadBufferSize  = 4096
	defaultWriteBufferSize = 4096
	defaultErrorHandler    = func(ctx *Ctx, err error) {
		code := StatusInternalServerError
		if e, ok := err.(*Error); ok {
			code = e.Code
		}
		ctx.Set(HeaderContentType, MIMETextPlainCharsetUTF8)
		ctx.Status(code).SendString(err.Error())
	}
	defaultCompressedFileSuffix = ".fiber.gz"
)

// New creates a new Fiber named instance.
// You can pass optional settings when creating a new instance.
func New(settings ...*Settings) *App {
	// Create a new app
	app := &App{
		// Create router stack
		stack: make([][]*Route, len(methodINT)),
		// Create Ctx pool
		pool: sync.Pool{
			New: func() interface{} {
				return new(Ctx)
			},
		},
		// Set settings
		Settings: &Settings{},
	}

	// Overwrite settings if provided
	if len(settings) > 0 {
		app.Settings = settings[0]
	}

	if app.Settings.BodyLimit <= 0 {
		app.Settings.BodyLimit = defaultBodyLimit
	}
	if app.Settings.Concurrency <= 0 {
		app.Settings.Concurrency = defaultConcurrency
	}
	if app.Settings.ReadBufferSize <= 0 {
		app.Settings.ReadBufferSize = defaultReadBufferSize
	}
	if app.Settings.WriteBufferSize <= 0 {
		app.Settings.WriteBufferSize = defaultWriteBufferSize
	}
	// Set default compressed file suffix
	if app.Settings.CompressedFileSuffix == "" {
		app.Settings.CompressedFileSuffix = defaultCompressedFileSuffix
	}
	// Set default error
	if app.Settings.ErrorHandler == nil {
		app.Settings.ErrorHandler = defaultErrorHandler
	}

	if !app.Settings.Prefork { // Default to -prefork flag if false
		app.Settings.Prefork = utils.GetArgument(flagPrefork)
	}
	// Replace unsafe conversion functions
	if app.Settings.Immutable {
		getBytes, getString = getBytesImmutable, getStringImmutable
	}

	// Return app
	return app
}

// Use registers a middleware route.
// Middleware matches requests beginning with the provided prefix.
// Providing a prefix is optional, it defaults to "/".
//
// - app.Use(handler)
// - app.Use("/api", handler)
// - app.Use("/api", handler, handler)
func (app *App) Use(args ...interface{}) *Route {
	var prefix string
	var handlers []Handler

	for i := 0; i < len(args); i++ {
		switch arg := args[i].(type) {
		case string:
			prefix = arg
		case Handler:
			handlers = append(handlers, arg)
		default:
			log.Fatalf("Use: Invalid Handler %v", reflect.TypeOf(arg))
		}
	}
	return app.register("USE", prefix, handlers...)
}

// Get ...
func (app *App) Get(path string, handlers ...Handler) *Route {
	return app.Add(MethodGet, path, handlers...)
}

// Head ...
func (app *App) Head(path string, handlers ...Handler) *Route {
	return app.Add(MethodHead, path, handlers...)
}

// Post ...
func (app *App) Post(path string, handlers ...Handler) *Route {
	return app.Add(MethodPost, path, handlers...)
}

// Put ...
func (app *App) Put(path string, handlers ...Handler) *Route {
	return app.Add(MethodPut, path, handlers...)
}

// Delete ...
func (app *App) Delete(path string, handlers ...Handler) *Route {
	return app.Add(MethodDelete, path, handlers...)
}

// Connect ...
func (app *App) Connect(path string, handlers ...Handler) *Route {
	return app.Add(MethodConnect, path, handlers...)
}

// Options ...
func (app *App) Options(path string, handlers ...Handler) *Route {
	return app.Add(MethodOptions, path, handlers...)
}

// Trace ...
func (app *App) Trace(path string, handlers ...Handler) *Route {
	return app.Add(MethodTrace, path, handlers...)
}

// Patch ...
func (app *App) Patch(path string, handlers ...Handler) *Route {
	return app.Add(MethodPatch, path, handlers...)
}

// Add ...
func (app *App) Add(method, path string, handlers ...Handler) *Route {
	return app.register(method, path, handlers...)
}

// Static ...
func (app *App) Static(prefix, root string, config ...Static) *Route {
	return app.registerStatic(prefix, root, config...)
}

// All ...
func (app *App) All(path string, handlers ...Handler) []*Route {
	routes := make([]*Route, len(methodINT))
	for method, i := range methodINT {
		routes[i] = app.Add(method, path, handlers...)
	}
	return routes
}

// Group is used for Routes with common prefix to define a new sub-router with optional middleware.
func (app *App) Group(prefix string, handlers ...Handler) *Group {
	if len(handlers) > 0 {
		app.register("USE", prefix, handlers...)
	}
	return &Group{prefix: prefix, app: app}
}

// Error makes it compatible with `error` interface.
func (e *Error) Error() string {
	return e.Message
}

// NewError creates a new HTTPError instance.
func NewError(code int, message ...string) *Error {
	e := &Error{code, utils.StatusMessage(code)}
	if len(message) > 0 {
		e.Message = message[0]
	}
	return e
}

// Routes returns all registered routes
//
// for _, r := range app.Routes() {
// 	fmt.Printf("%s\t%s\n", r.Method, r.Path)
// }
func (app *App) Routes() []*Route {
	routes := make([]*Route, 0)
	for m := range app.stack {
		for r := range app.stack[m] {
			// Ignore HEAD routes handling GET routes
			if m == 1 && app.stack[m][r].Method == MethodGet {
				continue
			}
			// Don't duplicate USE routes
			if app.stack[m][r].Method == "USE" {
				duplicate := false
				for i := range routes {
					if routes[i].Method == "USE" && routes[i].Name == app.stack[m][r].Name {
						duplicate = true
					}
				}
				if !duplicate {
					routes = append(routes, app.stack[m][r])
				}
			} else {
				routes = append(routes, app.stack[m][r])
			}
		}
	}
	// Sort routes by stack position
	sort.Slice(routes, func(i, k int) bool {
		return routes[i].pos < routes[k].pos
	})
	return routes
}

// Serve can be used to pass a custom listener
// This method does not support the Prefork feature
// Prefork is not supported using app.Serve(ln net.Listener)
// You can pass an optional *tls.Config to enable TLS.
func (app *App) Serve(ln net.Listener, tlsconfig ...*tls.Config) error {
	// Update fiber server settings
	app.init()
	// TLS config
	if len(tlsconfig) > 0 {
		ln = tls.NewListener(ln, tlsconfig[0])
	}
	// Print startup message
	if !app.Settings.DisableStartupMessage {
		app.startupMessage(ln.Addr().String())
	}

	return app.server.Serve(ln)
}

// Listen serves HTTP requests from the given addr or port.
// You can pass an optional *tls.Config to enable TLS.
func (app *App) Listen(address interface{}, tlsconfig ...*tls.Config) error {
	// Convert address to string
	addr, ok := address.(string)
	if !ok {
		port, ok := address.(int)
		if !ok {
			return fmt.Errorf("listen: host must be an `int` port or `string` address")
		}
		addr = strconv.Itoa(port)
	}
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}
	// Update fiber server settings
	app.init()
	// Print startup message
	if !app.Settings.DisableStartupMessage {
		app.startupMessage(addr)
	}
	// Start prefork
	if app.Settings.Prefork {
		return app.prefork(addr, tlsconfig...)
	}
	// Setup listener
	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		return err
	}
	// Add TLS config if provided
	if len(tlsconfig) > 0 {
		ln = tls.NewListener(ln, tlsconfig[0])
	}
	// Start listening
	return app.server.Serve(ln)
}

// Handler returns the server handler
func (app *App) Handler() fasthttp.RequestHandler {
	return app.handler
}

// Shutdown gracefully shuts down the server without interrupting any active connections.
// Shutdown works by first closing all open listeners and then waiting indefinitely for all connections to return to idle and then shut down.
//
// When Shutdown is called, Serve, ListenAndServe, and ListenAndServeTLS immediately return nil.
// Make sure the program doesn't exit and waits instead for Shutdown to return.
//
// Shutdown does not close keepalive connections so its recommended to set ReadTimeout to something else than 0.
func (app *App) Shutdown() error {
	app.mutex.Lock()
	defer app.mutex.Unlock()
	if app.server == nil {
		return fmt.Errorf("shutdown: server is not running")
	}
	return app.server.Shutdown()
}

// Test is used for internal debugging by passing a *http.Request
// Timeout is optional and defaults to 1s, -1 will disable it completely.
func (app *App) Test(request *http.Request, msTimeout ...int) (*http.Response, error) {
	timeout := 1000 // 1 second default
	if len(msTimeout) > 0 {
		timeout = msTimeout[0]
	}
	// Add Content-Length if not provided with body
	if request.Body != http.NoBody && request.Header.Get("Content-Length") == "" {
		request.Header.Add("Content-Length", strconv.FormatInt(request.ContentLength, 10))
	}
	// Dump raw http request
	dump, err := httputil.DumpRequest(request, true)
	if err != nil {
		return nil, err
	}
	// Update server settings
	app.init()
	// Create test connection
	conn := new(testConn)
	// Write raw http request
	if _, err = conn.r.Write(dump); err != nil {
		return nil, err
	}
	// Serve conn to server
	channel := make(chan error)
	go func() {
		channel <- app.server.ServeConn(conn)
	}()
	// Wait for callback
	if timeout >= 0 {
		// With timeout
		select {
		case err = <-channel:
		case <-time.After(time.Duration(timeout) * time.Millisecond):
			return nil, fmt.Errorf("test: timeout error %vms", timeout)
		}
	} else {
		// Without timeout
		err = <-channel
	}
	// Check for errors
	if err != nil {
		return nil, err
	}
	// Read response
	buffer := bufio.NewReader(&conn.w)
	// Convert raw http response to *http.Response
	resp, err := http.ReadResponse(buffer, request)
	if err != nil {
		return nil, err
	}
	// Return *http.Response
	return resp, nil
}

type disableLogger struct{}

func (dl *disableLogger) Printf(format string, args ...interface{}) {
	// fmt.Println(fmt.Sprintf(format, args...))
}

func (app *App) init() *App {
	app.mutex.Lock()
	// Load view engine if provided
	if app.Settings != nil {
		// Templates is replaced by Views with layout support
		if app.Settings.Templates != nil {
			fmt.Println("`Templates` are deprecated since v1.12.x, please us `Views` instead")
		}
		// Only load templates if an view engine is specified
		if app.Settings.Views != nil {
			if err := app.Settings.Views.Load(); err != nil {
				fmt.Printf("views: %v\n", err)
			}
		}
	}
	if app.server == nil {
		app.server = &fasthttp.Server{
			Logger:       &disableLogger{},
			LogAllErrors: false,
			ErrorHandler: func(fctx *fasthttp.RequestCtx, err error) {
				ctx := app.AcquireCtx(fctx)
				if _, ok := err.(*fasthttp.ErrSmallBuffer); ok {
					ctx.err = ErrRequestHeaderFieldsTooLarge
				} else if netErr, ok := err.(*net.OpError); ok && netErr.Timeout() {
					ctx.err = ErrRequestTimeout
				} else if len(err.Error()) == 33 && err.Error() == "body size exceeds the given limit" {
					ctx.err = ErrRequestEntityTooLarge
				} else {
					ctx.err = ErrBadRequest
				}
				app.Settings.ErrorHandler(ctx, ctx.err)
				app.ReleaseCtx(ctx)
			},
		}
	}
	if app.server.Handler == nil {
		app.server.Handler = app.handler
	}
	app.server.Name = app.Settings.ServerHeader
	app.server.Concurrency = app.Settings.Concurrency
	app.server.NoDefaultDate = app.Settings.DisableDefaultDate
	app.server.NoDefaultContentType = app.Settings.DisableDefaultContentType
	app.server.DisableHeaderNamesNormalizing = app.Settings.DisableHeaderNormalizing
	app.server.DisableKeepalive = app.Settings.DisableKeepalive
	app.server.MaxRequestBodySize = app.Settings.BodyLimit
	app.server.NoDefaultServerHeader = app.Settings.ServerHeader == ""
	app.server.ReadTimeout = app.Settings.ReadTimeout
	app.server.WriteTimeout = app.Settings.WriteTimeout
	app.server.IdleTimeout = app.Settings.IdleTimeout
	app.server.ReadBufferSize = app.Settings.ReadBufferSize
	app.server.WriteBufferSize = app.Settings.WriteBufferSize
	app.mutex.Unlock()
	return app
}

const (
	cBlack = "\u001b[90m"
	// cRed     = "\u001b[91m"
	cGreen = "\u001b[92m"
	// cYellow  = "\u001b[93m"
	// cBlue    = "\u001b[94m"
	// cMagenta = "\u001b[95m"
	// cCyan    = "\u001b[96m"
	// cWhite   = "\u001b[97m"
	cReset = "\u001b[0m"
)

func (app *App) startupMessage(port string) {
	// tabwriter makes sure the spacing are consistant across different values
	// colorable handles the escape sequence for stdout using ascii color codes
	out := tabwriter.NewWriter(colorable.NewColorableStdout(), 0, 8, 4, ' ', 0)
	if !utils.GetArgument(flagChild) {
		fmt.Fprintf(out, "%s        _______ __\n  ____ / ____(_) /_  ___  _____\n_____ / /_  / / __ \\/ _ \\/ ___/\n  __ / __/ / / /_/ /  __/ /\n    /_/   /_/_.___/\\___/_/", cGreen)
		fmt.Fprintf(out, "%s v%s\n\n", cBlack, Version)
		fmt.Fprintf(out, "PORT: %s%s%s \tROUTES:  %s%v%s\n", cGreen, port, cBlack, cGreen, len(app.Routes()), cBlack)
		fmt.Fprintf(out, "PPID: %s%v%s \tPREFORK: %s%v%s\n", cGreen, os.Getppid(), cBlack, cGreen, app.Settings.Prefork, cBlack)
		fmt.Fprintf(out, "OS:   %s%v%s \tARCH:    %s%v%s\n\n", cGreen, runtime.GOOS, cBlack, cGreen, runtime.GOARCH, cReset)
	}
	_ = out.Flush()
}
