package main

import (
	"net/http"
	"os"
	"path"
)

func (cfg *apiConfig) handlerThumbnailGet(w http.ResponseWriter, r *http.Request) {
	// Get the filename from the path
	filename := r.PathValue("filename")

	// Construct the full file path
	filePath := path.Join(cfg.assetsRoot, filename)

	// Check if file exists
	_, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			respondWithError(w, http.StatusNotFound, "Thumbnail not found", err)
			return
		}
		respondWithError(w, http.StatusInternalServerError, "Error checking thumbnail", err)
		return
	}

	// Serve the file
	http.ServeFile(w, r, filePath)
}
