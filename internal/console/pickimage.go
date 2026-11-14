package console

import (
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/chenhg5/myco/internal/client"
)

// maxPicture is the largest picture read for a face (it is made small in
// the page before it is sent anywhere).
const maxPicture = 20 << 20

// pictureTypes are the pictures a face can be made from.
var pictureTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true, "image/gif": true}

// pickImage: POST /app/pick-image → {cancelled} or {name, type, data
// (base64)}. Only what is really a picture comes back, whatever its name.
func (b *Backend) pickImage(w http.ResponseWriter, r *http.Request) {
	if b.PickImage == nil {
		writeErr(w, &client.Error{Code: "not_supported", Message: "only the desktop app has its own picture panel", Status: http.StatusNotFound})
		return
	}
	path, err := b.PickImage()
	if err != nil {
		writeErr(w, &client.Error{Code: "unavailable", Message: "could not open the picture panel", Status: http.StatusConflict})
		return
	}
	if path == "" {
		writeJSON(w, http.StatusOK, map[string]bool{"cancelled": true})
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeErr(w, &client.Error{Code: "not_found", Message: "could not read that file", Status: http.StatusNotFound})
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxPicture+1))
	if err != nil {
		writeErr(w, &client.Error{Code: "not_found", Message: "could not read that file", Status: http.StatusNotFound})
		return
	}
	if len(data) > maxPicture {
		writeErr(w, &client.Error{Code: "too_large", Message: "the picture is larger than 20 MB", Status: http.StatusRequestEntityTooLarge})
		return
	}
	kind := http.DetectContentType(data)
	if !pictureTypes[kind] {
		writeErr(w, &client.Error{Code: "not_image", Message: "only a picture can be a face: PNG, JPEG, WebP or GIF", Status: http.StatusUnsupportedMediaType})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": filepath.Base(path), "type": kind, "data": base64.StdEncoding.EncodeToString(data)})
}
