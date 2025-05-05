package main

import (
	"fmt"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"io"
	"net/http"
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

	// TODO: implement the upload here
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
	}
	data, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not get video", err)
	}
	if data.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You are not authorized to upload this thumbnail", err)
	}
	tb := thumbnail{data: imageData, mediaType: fileHeader.Header.Get("Content-Type")}
	//respondWithJSON(w, http.StatusOK, struct{}{})
	videoThumbnails[videoID] = tb
	thumbnailURL := "http://localhost:8091/api/thumbnails/" + videoID.String()
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
