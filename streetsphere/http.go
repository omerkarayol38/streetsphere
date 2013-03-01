package streetsphere

import (
	"archive/zip"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"time"

	"appengine"
)

var (
	outputTemplate = template.Must(template.ParseFiles("output.html"))
	indexPage      []byte
)

func init() {
	http.Handle("/", errorHandler(rootHandler))
	http.Handle("/upload", errorHandler(uploadHandler))

	var err error
	indexPage, err = ioutil.ReadFile("index.html")
	if err != nil {
		panic(err)
	}
}

// rootHandler sends an upload form.
func rootHandler(c appengine.Context, w http.ResponseWriter, r *http.Request) *appError {
	_, err := w.Write(indexPage)
	if err != nil {
		logError(c, "couldn't write index page", err)
	}
	return nil
}

// uploadHandler retreives the image provided by the user, pads the image,
// generates a HTML file, then stores both files within a ZIP, which is then
// sent in the response.
func uploadHandler(c appengine.Context, w http.ResponseWriter, r *http.Request) *appError {
	fn := fmt.Sprintf("photosphere-streetview-%d", time.Now().Unix())
	w.Header().Add("Content-Disposition", fmt.Sprintf(`attachment;filename="%s.zip"`, fn))

	err := r.ParseMultipartForm(10 << 20) // 10 MiB limit
	if err != nil {
		return appErrorf(err, "couldn't parse form")
	}

	b := r.MultipartForm.File["img"]
	if len(b) < 1 {
		return appErrorf(nil, "could not find image in upload")
	}
	img := b[0]

	ir, err := img.Open()
	if err != nil {
		return appErrorf(err, "couldn't read image from upload")
	}

	zw := zip.NewWriter(w)
	iw, err := zw.Create(fmt.Sprintf("%s/%s", fn, img.Filename))
	if err != nil{
		return appErrorf(err, "couldn't create image in zip")
	}

	pano, err := Pad(iw, ir)
	if err != nil {
		return appErrorf(err, "couldn't convert image to street view format")
	}

	hw, err := zw.Create(fmt.Sprintf("%s/streetview.html", fn))
	if err != nil {
		return appErrorf(err, "couldn't create index file in zip")
	}

	header := fmt.Sprintf("<!-- Generated by %s.appspot.com -->", appengine.AppID(c))
	err = outputTemplate.Execute(hw, struct {
		ImageFilename string
		Pano          *PanoOpts
		Header        template.HTML
	}{img.Filename, pano, template.HTML(header)})
	if err != nil {
		return appErrorf(err, "couldn't write index file")
	}

	err = zw.Close()
	if err != nil {
		return appErrorf(err, "couldn't create zip file")
	}
	return nil
}

func logError(c appengine.Context, msg string, err error) {
	c.Errorf("%s (%v)", msg, err)
}

type errorHandler func(appengine.Context, http.ResponseWriter, *http.Request) *appError

func (handler errorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if e := handler(c, w, r); e != nil {
		http.Error(w, e.Message, e.Code)
		logError(c, e.Message, e.Error)
	}
}

type appError struct {
	Error   error
	Message string
	Code    int
}

func appErrorf(err error, format string, v ...interface{}) *appError {
	return &appError{err, fmt.Sprintf(format, v...), 500}
}
