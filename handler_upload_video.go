package main

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
	"io"
	"math"
	"mime"
	"net/http"
	"os"
	"os/exec"
)

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-print_format", "json", "-show_streams", filePath)
	buffer := bytes.NewBuffer(nil)
	cmd.Stdout = buffer
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	//unmarshall into a JSON struct
	type Stream struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	type VideoData struct {
		Streams []Stream `json:"streams"`
	}
	var decodedData VideoData
	err = json.NewDecoder(buffer).Decode(&decodedData)
	if err != nil {
		return "", err
	}
	ratio := float64(decodedData.Streams[0].Width) / float64(decodedData.Streams[0].Height)
	epsilon := 0.01
	ratio169 := 16.0 / 9.0
	ratio916 := 9.01 / 16.0
	switch {
	case math.Abs(ratio-ratio169) < epsilon:
		return "16:9", nil
	case math.Abs(ratio-ratio916) < epsilon:
		return "9:16", nil
	default:
		return "Other", nil
	}
}

func processVideoForFastStart(filePath string) (string, error) {
	outputFilepath := filePath + ".procceessing"
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFilepath)
	fmt.Printf("cmd is %v\n", cmd)
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return outputFilepath, nil
}

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	http.MaxBytesReader(w, r.Body, 1<<30)
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Video ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Printf("Uploading Video for ID %v by user %v\n", videoID, userID)

	videoData, err := cfg.db.GetVideo(videoID)
	if errors.Is(err, sql.ErrNoRows) {
		respondWithError(w, http.StatusBadRequest, "Could not find video", err)
		return
	} else if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not get video", err)
		return
	}

	videoUploader := videoData.UserID
	if videoUploader != userID {
		respondWithError(w, http.StatusUnauthorized, "You are not allowed to upload this video", err)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Could not get file", err)
		return
	}
	defer file.Close()
	contentType := header.Header.Get("Content-Type")
	mimeType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type header", err)
		return
	}
	if mimeType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "File is not an mp4 file", err)
		return
	}
	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create temp file", err)
		return
	}
	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not save video", err)
		return
	}
	tempFile.Seek(0, io.SeekStart)
	// TODO use processVideoForFastStart and copy the fast start version instead of the full file

	processedVideoPath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not process video", err)
		return
	}

	processedVideo, err := os.Open(processedVideoPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create processed video file", err)
		return
	}
	defer os.Remove(processedVideoPath)
	defer processedVideo.Close()

	_, err = processedVideo.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not seek video", err)
	}

	randomBytes := make([]byte, 32)
	_, err = rand.Read(randomBytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not generate random bytes", err)
		return
	}
	ratio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not get video aspect ratio", err)
		return
	}
	var aspectBucket string
	switch ratio {
	case "16:9":
		aspectBucket = "landscape"
	case "9:16":
		aspectBucket = "portrait"
	default:
		aspectBucket = "other"
	}
	hexStr := hex.EncodeToString(randomBytes)
	uploadFilename := aspectBucket + "/" + hexStr + ".mp4"
	s3Url := fmt.Sprintf("https://%v.s3.%v.amazonaws.com/%v", cfg.s3Bucket, cfg.s3Region, uploadFilename)
	fmt.Println(s3Url)

	putObjectInput := &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &uploadFilename,
		Body:        processedVideo,
		ContentType: aws.String("video/mp4"),
	}
	_, err = cfg.s3Client.PutObject(r.Context(), putObjectInput)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not upload video", err)
		return
	}

	//video := database.Video{ID: videoID, VideoURL: &s3Url}
	videoData.VideoURL = &s3Url
	err = cfg.db.UpdateVideo(videoData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err)
		return
	}
}
