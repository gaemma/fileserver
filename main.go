package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ResponseWriter wraps the http.ResponseWriter.
type ResponseWriter struct {
	inner  http.ResponseWriter
	status int
}

func (w *ResponseWriter) Header() http.Header {
	return w.inner.Header()
}

func (w *ResponseWriter) WriteHeader(code int) {
	w.status = code
	w.inner.WriteHeader(code)
}

func (w *ResponseWriter) Write(b []byte) (int, error) {
	return w.inner.Write(b)
}

var (
	root  string
	file  string
	netIF string
	port  int
)

func init() {
	flag.StringVar(&root, "f", "", "root directory or a file allowed to be visited.")
	flag.IntVar(&port, "p", 8000, "http server port")
	flag.StringVar(&netIF, "i", "", "net interface")
	flag.Parse()
}

func main() {
	err := resolvePath()
	if err != nil {
		exit(err)
	}

	ip, err := resolveInterface()
	if err != nil {
		exit(err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		pw := &ResponseWriter{w, 200}
		defer func() {
			ip := r.RemoteAddr
			if strings.Contains(ip, ":") {
				ip = strings.Split(ip, ":")[0]
			}
			log(strings.Join([]string{"[" + ip + "]", strconv.Itoa(pw.status), r.URL.Path}, " "))
		}()
		serve(pw, r)
	})
	addr := ip + ":" + strconv.Itoa(port)
	log("serveing on http://%s.", addr)
	err = http.ListenAndServe(addr, nil)
	if err != nil {
		exit(err)
	}
}

func serve(w http.ResponseWriter, r *http.Request) {
	// 根目录必然允许访问
	if r.URL.Path != "/" {
		// 如果 file 是 *，则代表可以访问根目录下所有文件。否则，只能访问根目录下某个文件
		if file != "*" && strings.Trim(r.URL.Path, "/") != file {
			http.NotFound(w, r)
			return
		}
	}
	query := filepath.Join(root, r.URL.Path)
	fi, err := os.Stat(query)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		log("fail to stat file %s: %s.", query, err.Error())
		internalServerError(w)
		return
	}
	if fi.IsDir() {
		var files []string
		if file == "*" {
			if r.URL.Path != "/" {
				files = append(files, "..")
			}
			fis, err := ioutil.ReadDir(query)
			if err != nil {
				internalServerError(w)
				return
			}
			for _, fi := range fis {
				files = append(files, fi.Name())
			}
		} else {
			files = append(files, file)
		}
		err := listFiles(w, r, files)
		if err != nil {
			log("fail to list files %s: %s.", query, err.Error())
		}
	} else {
		f, err := os.Open(query)
		if err != nil {
			log("fail to open file %s: %s.", query, err.Error())
			internalServerError(w)
			return
		}
		http.ServeContent(w, r, "foo", fi.ModTime(), f)
		err = f.Close()
		if err != nil {
			log("fail to close %s: %s.", query, err.Error())
		}
	}
}

func resolvePath() error {
	if len(root) == 0 {
		root, _ = os.Getwd()
	}
	fi, err := os.Stat(root)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		root, err = filepath.Abs(root)
		file = "*"
	} else {
		abs, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		root = filepath.Dir(abs)
		file = filepath.Base(abs)
	}
	return nil
}

func resolveInterface() (string, error) {
	var (
		n   *net.Interface
		err error
	)
	if len(netIF) > 0 {
		n, err = net.InterfaceByName(netIF)
	} else {
		n, err = net.InterfaceByIndex(1)
	}
	if err != nil {
		return "", err
	}

	addrs, err := n.Addrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		switch v := addr.(type) {
		case *net.IPNet:
			if v.IP.To4() != nil {
				log("choosing interface: %s.", n.Name)
				return v.IP.String(), nil
			}
		default:
		}
	}
	return "", net.InvalidAddrError("fail to find addr")
}

func exit(msg interface{}) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(10)
}

func log(format string, a ...interface{}) {
	if !strings.HasSuffix(format, "\n") {
		format += "\n"
	}
	format = time.Now().Format("2006.01.02 15:04:05 ") + format
	fmt.Fprintf(os.Stderr, format, a...)
}

func internalServerError(w http.ResponseWriter) {
	http.Error(w, "internal server error.", 500)
}

func listFiles(w http.ResponseWriter, r *http.Request, files []string) error {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>%s</title>
</head>
<body>
<h1>Index of %s</h1>
<hr>
<p>
%s</p>
</body>
</html>`
	buf := new(bytes.Buffer)
	for _, file := range files {
		buf.WriteString(fmt.Sprintf("<a href=\"%s\">%s</a><br>\n", filepath.Join(r.URL.Path, file), file))
	}
	_, err := w.Write([]byte(fmt.Sprintf(html, r.URL.Path, r.URL.Path, buf.String())))
	return err
}
