package main

import drive "google.golang.org/api/drive/v3"

type Folder struct {
	Service *drive.Service
	ID      string
	Mail    string
}
