package common

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

// P2E panic to error
func P2E(perr *error) {
	if x := recover(); x != nil {
		if e, ok := x.(error); ok {
			*perr = e
		} else {
			panic(x)
		}
	}
}

// E2P error to panic
func E2P(err error) {
	if err != nil {
		panic(err)
	}
}

// CatchP catch panic to error
func CatchP(f func()) (rerr error) {
	defer P2E(&rerr)
	f()
	return nil
}

// PanicIf name is clear
func PanicIf(cond bool, err error) {
	if cond {
		panic(err)
	}
}

// MustAtoi 走must逻辑
func MustAtoi(s string) int {
	r, err := strconv.Atoi(s)
	if err != nil {
		E2P(errors.New("convert to int error: " + s))
	}
	return r
}

// OrString return the first not empty string
func OrString(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// If ternary operator
func If(condition bool, trueObj interface{}, falseObj interface{}) interface{} {
	if condition {
		return trueObj
	}
	return falseObj
}

// MustMarshal checked version for marshal
func MustMarshal(v interface{}) []byte {
	b, err := json.Marshal(v)
	E2P(err)
	return b
}

// MustMarshalString string version of MustMarshal
func MustMarshalString(v interface{}) string {
	return string(MustMarshal(v))
}

// MustUnmarshal checked version for unmarshal
func MustUnmarshal(b []byte, obj interface{}) {
	err := json.Unmarshal(b, obj)
	E2P(err)
}

// MustUnmarshalString string version of MustUnmarshal
func MustUnmarshalString(s string, obj interface{}) {
	MustUnmarshal([]byte(s), obj)
}

// MustRemarshal marshal and unmarshal, and check error
func MustRemarshal(from interface{}, to interface{}) {
	b, err := json.Marshal(from)
	E2P(err)
	err = json.Unmarshal(b, to)
	E2P(err)
}

// GetGinApp init and return gin
func GetGinApp() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	app := gin.Default()
	app.Use(func(c *gin.Context) {
		body := ""
		if c.Request.Body != nil {
			rb, err := c.GetRawData()
			E2P(err)
			if len(rb) > 0 {
				body = string(rb)
				c.Request.Body = ioutil.NopCloser(bytes.NewBuffer(rb))
			}
		}
		began := time.Now()
		logrus.Printf("begin %s %s query: %s body: %s", c.Request.Method, c.FullPath(), c.Request.URL.RawQuery, body)
		c.Next()
		logrus.Printf("used %d ms %s %s query: %s body: %s", time.Since(began).Milliseconds(), c.Request.Method, c.FullPath(), c.Request.URL.RawQuery, body)

	})
	app.Any("/api/ping", func(c *gin.Context) { c.JSON(200, M{"msg": "pong"}) })
	return app
}

// WrapHandler name is clear
func WrapHandler(fn func(*gin.Context) (interface{}, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		r, err := fn(c)
		var b = []byte{}
		if err == nil {
			b, err = json.Marshal(r)
		}
		if err != nil {
			logrus.Printf("status: 500, code: 500 message: %s", err.Error())
			c.JSON(500, M{"code": 500, "message": err.Error()})
		} else {
			logrus.Printf("status: 200, content: %s", string(b))
			c.Status(200)
			c.Writer.Header().Add("Content-Type", "application/json")
			_, err = c.Writer.Write(b)
			E2P(err)
		}
	}
}

// RestyClient the resty object
var RestyClient = resty.New()

func init() {
	// RestyClient.SetTimeout(3 * time.Second)
	// RestyClient.SetRetryCount(2)
	// RestyClient.SetRetryWaitTime(1 * time.Second)
	RestyClient.OnBeforeRequest(func(c *resty.Client, r *resty.Request) error {
		if IsDockerCompose() {
			r.URL = strings.Replace(r.URL, "localhost", "host.docker.internal", 1)
		}
		logrus.Printf("requesting: %s %s %v %v", r.Method, r.URL, r.Body, r.QueryParam)
		return nil
	})
	RestyClient.OnAfterResponse(func(c *resty.Client, resp *resty.Response) error {
		r := resp.Request
		logrus.Printf("requested: %s %s %s", r.Method, r.URL, resp.String())
		return nil
	})
}

// CheckRestySuccess panic if error or resp not success
func CheckRestySuccess(resp *resty.Response, err error) {
	E2P(err)
	if !strings.Contains(resp.String(), "SUCCESS") {
		panic(fmt.Errorf("resty response not success: %s", resp.String()))
	}
}

// formatter 自定义formatter
type formatter struct{}

// Format 进行格式化
func (f *formatter) Format(entry *logrus.Entry) ([]byte, error) {
	var b *bytes.Buffer = &bytes.Buffer{}
	if entry.Buffer != nil {
		b = entry.Buffer
	}
	n := time.Now()
	ts := fmt.Sprintf("%d-%02d-%02d %02d:%02d:%02d.%03d", n.Year(), n.Month(), n.Day(), n.Hour(), n.Minute(), n.Second(), n.Nanosecond()/1000000)
	var file string
	var line int
	for i := 1; ; i++ {
		_, file, line, _ = runtime.Caller(i)
		if strings.Contains(file, "dtm") {
			break
		}
	}
	b.WriteString(fmt.Sprintf("%s %s:%d %s\n", ts, path.Base(file), line, entry.Message))
	return b.Bytes(), nil
}

// InitApp init config
func InitApp(dir string, config interface{}) {
	logrus.SetFormatter(&formatter{})
	cont, err := ioutil.ReadFile(dir + "/conf.yml")
	if err != nil {
		cont, err = ioutil.ReadFile(dir + "/conf.sample.yml")
	}
	logrus.Printf("cont is: \n%s", string(cont))
	E2P(err)
	err = yaml.Unmarshal(cont, config)
	E2P(err)
}

// MustGetwd must version of os.Getwd
func MustGetwd() string {
	wd, err := os.Getwd()
	E2P(err)
	return wd
}

// GetCurrentCodeDir name is clear
func GetCurrentCodeDir() string {
	_, file, _, _ := runtime.Caller(1)
	return filepath.Dir(file)
}

// GetProjectDir name is clear
func GetProjectDir() string {
	_, file, _, _ := runtime.Caller(1)
	for ; !strings.HasSuffix(file, "/dtm"); file = filepath.Dir(file) {
	}
	return file
}

// GetFuncName get current call func name
func GetFuncName() string {
	pc, _, _, _ := runtime.Caller(1)
	return runtime.FuncForPC(pc).Name()
}

// IsDockerCompose name is clear
func IsDockerCompose() bool {
	return os.Getenv("IS_DOCKER_COMPOSE") != ""
}
