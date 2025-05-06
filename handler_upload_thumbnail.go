package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		respondWithError(w, http.StatusBadRequest, "Could not parse multipart form", err)
		return
	}
	file, fileHeader, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Could not read thumbnail file", err)
		return
	}
	defer file.Close()
	fmt.Printf("Uploaded File: %+v\n", fileHeader)
	fmt.Printf("File Size: %+v\n", fileHeader.Size)
	fmt.Printf("MIME Header: %+v\n", fileHeader.Header)
	var imageData []byte
	imageData, err = io.ReadAll(file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Could not read thumbnail file", err)
		return
	}
	data, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not get video", err)
		return
	}
	if data.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You are not authorized to upload this thumbnail", err)
		return
	}
	tb := thumbnail{data: imageData, mediaType: fileHeader.Header.Get("Content-Type")}
	randomBits := make([]byte, 32)
	rand.Read(randomBits)
	randomString := base64.RawURLEncoding.EncodeToString(randomBits)
	videoThumbnails[videoID] = tb
	//thumbnailURL := "http://localhost:8091/api/thumbnails/" + videoID.String()
	extensions, err := mime.ExtensionsByType(tb.mediaType)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not get extension", err)
		return
	}
	if len(extensions) == 0 {
		respondWithError(w, http.StatusInternalServerError, "No extensions found for media type", nil)
		return
	}
	thumbnailURL := "http://localhost:8091/api/thumbnails/" + randomString + extensions[0]
	err = os.WriteFile(path.Join(cfg.assetsRoot, randomString+extensions[0]), imageData, 0644)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not upload thumbnail", err)
		return
	}

	video := database.Video{
		ID:                data.ID,
		CreatedAt:         data.CreatedAt,
		UpdatedAt:         time.Now(),
		ThumbnailURL:      &thumbnailURL,
		VideoURL:          data.VideoURL,
		CreateVideoParams: data.CreateVideoParams,
	}
	cfg.db.UpdateVideo(video)
	respondWithJSON(w, 200, video)
}
