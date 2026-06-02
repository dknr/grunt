package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"grunt/server/storage"
)

// setupAvatarTest creates a store with one user and returns the store + userID.
func setupAvatarTest(t *testing.T) (*storage.Store, string) {
	t.Helper()
	store, err := storage.New("file::memory:?cache=shared")
	if err != nil {
		t.Fatal("create store:", err)
	}
	t.Cleanup(func() { store.Close() })

	if err := store.CreateUser("avataruser", "pass"); err != nil {
		t.Fatal("create user:", err)
	}
	if err := store.SetUserAdmin("avataruser"); err != nil {
		t.Fatal("set admin:", err)
	}
	return store, "avataruser"
}

// createTestPNGBytes returns a 100×100 solid blue PNG as bytes.
func createTestPNGBytes() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

// newAuthCtx returns a context with auth values matching auth.go keys.
func newAuthCtx(userID string, authenticated, isAdmin bool) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, userIDKey, userID)
	ctx = context.WithValue(ctx, isAdminKey, isAdmin)
	ctx = context.WithValue(ctx, authenticatedKey, authenticated)
	return ctx
}

func TestHandleAvatarUpload_Success(t *testing.T) {
	store, userID := setupAvatarTest(t)
	api := &apiImpl{store: store}

	pngData := createTestPNGBytes()
	var body bytes.Buffer
	body.WriteString("--boundary\r\nContent-Disposition: form-data; name=\"avatar\"; filename=\"test.png\"\r\nContent-Type: image/png\r\n\r\n")
	body.Write(pngData)
	body.WriteString("\r\n--boundary--\r\n")

	req := httptest.NewRequest("POST", "/api/user/avatar", &body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	req = req.WithContext(newAuthCtx(userID, true, true))

	w := httptest.NewRecorder()
	api.handleAvatarUpload(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal("decode response:", err)
	}
	if result["status"] != "ok" {
		t.Errorf("expected status ok, got %s", result["status"])
	}
	if !store.HasAvatar(userID) {
		t.Error("avatar should exist after upload")
	}
}

func TestHandleAvatarUpload_Unauthenticated(t *testing.T) {
	store, _ := setupAvatarTest(t)
	api := &apiImpl{store: store}

	pngData := createTestPNGBytes()
	var body bytes.Buffer
	body.WriteString("--boundary\r\nContent-Disposition: form-data; name=\"avatar\"; filename=\"test.png\"\r\nContent-Type: image/png\r\n\r\n")
	body.Write(pngData)
	body.WriteString("\r\n--boundary--\r\n")

	req := httptest.NewRequest("POST", "/api/user/avatar", &body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	// No auth context

	w := httptest.NewRecorder()
	api.handleAvatarUpload(w, req)
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Result().StatusCode)
	}
}

func TestHandleAvatarUpload_NoFile(t *testing.T) {
	store, userID := setupAvatarTest(t)
	api := &apiImpl{store: store}

	var body bytes.Buffer
	body.WriteString("--boundary\r\n--boundary--\r\n")

	req := httptest.NewRequest("POST", "/api/user/avatar", &body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	req = req.WithContext(newAuthCtx(userID, true, true))

	w := httptest.NewRecorder()
	api.handleAvatarUpload(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Result().StatusCode)
	}
}

func TestHandleAvatarUpload_InvalidImage(t *testing.T) {
	store, userID := setupAvatarTest(t)
	api := &apiImpl{store: store}

	var body bytes.Buffer
	body.WriteString("--boundary\r\nContent-Disposition: form-data; name=\"avatar\"; filename=\"bad.txt\"\r\nContent-Type: text/plain\r\n\r\nnot an image\r\n--boundary--\r\n")

	req := httptest.NewRequest("POST", "/api/user/avatar", &body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	req = req.WithContext(newAuthCtx(userID, true, true))

	w := httptest.NewRecorder()
	api.handleAvatarUpload(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Result().StatusCode)
	}
}

func TestHandleAvatarGet_Success(t *testing.T) {
	store, userID := setupAvatarTest(t)
	api := &apiImpl{store: store}

	pngData := createTestPNGBytes()
	if err := store.SetAvatar(userID, pngData); err != nil {
		t.Fatal("set avatar:", err)
	}

	req := httptest.NewRequest("GET", "/api/user/avatar", nil)
	req = req.WithContext(newAuthCtx(userID, true, true))

	w := httptest.NewRecorder()
	api.handleAvatarGet(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "image/png" {
		t.Errorf("expected image/png, got %s", resp.Header.Get("Content-Type"))
	}
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, pngData) {
		t.Error("avatar bytes mismatch")
	}
}

func TestHandleAvatarGet_NoAvatar(t *testing.T) {
	store, userID := setupAvatarTest(t)
	api := &apiImpl{store: store}

	req := httptest.NewRequest("GET", "/api/user/avatar", nil)
	req = req.WithContext(newAuthCtx(userID, true, true))

	w := httptest.NewRecorder()
	api.handleAvatarGet(w, req)
	if w.Result().StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Result().StatusCode)
	}
}

func TestHandleAvatarGet_Unauthenticated(t *testing.T) {
	store, _ := setupAvatarTest(t)
	api := &apiImpl{store: store}

	req := httptest.NewRequest("GET", "/api/user/avatar", nil)
	w := httptest.NewRecorder()
	api.handleAvatarGet(w, req)
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Result().StatusCode)
	}
}

func TestHandleAvatarGetUser_Success(t *testing.T) {
	store, userID := setupAvatarTest(t)
	api := &apiImpl{store: store}

	pngData := createTestPNGBytes()
	if err := store.SetAvatar(userID, pngData); err != nil {
		t.Fatal("set avatar:", err)
	}

	req := httptest.NewRequest("GET", "/api/user/avatar/"+userID, nil)
	w := httptest.NewRecorder()
	api.handleAvatarGetUser(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "image/png" {
		t.Errorf("expected image/png, got %s", resp.Header.Get("Content-Type"))
	}
}

func TestHandleAvatarGetUser_NotFound(t *testing.T) {
	store, _ := setupAvatarTest(t)
	api := &apiImpl{store: store}

	req := httptest.NewRequest("GET", "/api/user/avatar/nonexistent", nil)
	w := httptest.NewRecorder()
	api.handleAvatarGetUser(w, req)
	if w.Result().StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Result().StatusCode)
	}
}

func TestHandleAvatarGetUser_EmptyUserID(t *testing.T) {
	store, _ := setupAvatarTest(t)
	api := &apiImpl{store: store}

	req := httptest.NewRequest("GET", "/api/user/avatar/", nil)
	w := httptest.NewRecorder()
	api.handleAvatarGetUser(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Result().StatusCode)
	}
}

func TestHandleAvatarGetUser_NoAvatar(t *testing.T) {
	store, userID := setupAvatarTest(t)
	api := &apiImpl{store: store}

	req := httptest.NewRequest("GET", "/api/user/avatar/"+userID, nil)
	w := httptest.NewRecorder()
	api.handleAvatarGetUser(w, req)
	if w.Result().StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Result().StatusCode)
	}
}
