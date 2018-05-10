package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/fatih/color"
	"golang.org/x/oauth2"
	drive "google.golang.org/api/drive/v3"
)

var (
	from       = flag.String("from", "", "Source email address")
	to         = flag.String("to", "", "Destination email address")
	fromFolder = flag.String("fromFolder", "", "Source Folder")
	toFolder   = flag.String("toFolder", "", "Destination Folder")
	conf       *oauth2.Config
	ctx        context.Context
)

func saveToken(mail string, token *oauth2.Token) error {
	tokenJson, _ := json.Marshal(token)
	return ioutil.WriteFile(fmt.Sprintf("%s.json", mail), tokenJson, 0400)
}

func loadToken(mail string) (token *oauth2.Token, err error) {
	content, err := ioutil.ReadFile(fmt.Sprintf("%s.json", mail))
	if err != nil {
		log.Printf(color.RedString("Unable to load token: %s"), err)
		return nil, err
	}

	err = json.Unmarshal(content, &token)
	if err != nil {
		log.Printf(color.RedString("Unable to load token: %s"), err)
		return nil, err
	}
	if !token.Valid() {
		removeAuth(mail)
		return nil, fmt.Errorf("Token invalid")
	}
	return token, nil
}

func removeAuth(mail string) error {
	return os.Remove(fmt.Sprintf("%s.json", mail))
}

func auth(mail string) *http.Client {
	tok, err := loadToken(mail)
	if err != nil {
		conf = &oauth2.Config{
			ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
			ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
			Scopes:       []string{"openid", "profile", drive.DriveScope, drive.DriveMetadataScope, drive.DriveFileScope},
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://accounts.google.com/o/oauth2/auth",
				TokenURL: "https://accounts.google.com/o/oauth2/token",
			},
			// my own callback URL
			RedirectURL: "urn:ietf:wg:oauth:2.0:oob",
		}

		// add transport for self-signed certificate to context
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		sslcli := &http.Client{Transport: tr}
		ctx = context.WithValue(ctx, oauth2.HTTPClient, sslcli)
		// Redirect user to consent page to ask for permission
		// for the scopes specified above.
		url := conf.AuthCodeURL("state", oauth2.AccessTypeOffline)

		log.Printf(color.CyanString("You will now be taken to your browser for authentication for %s"), mail)
		log.Printf("Authentication URL: %s\n", url)
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter code: ")
		code, _ := reader.ReadString('\n')

		// Exchange will do the handshake to retrieve the initial access token.
		tok, err = conf.Exchange(ctx, code)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Token: %s", tok)
		if err := saveToken(mail, tok); err != nil {
			log.Printf(color.RedString("Unable to save token, continuing: %s"), err)
		}
	}
	// The HTTP Client returned by conf.Client will refresh the token as necessary.
	client := conf.Client(ctx, tok)

	resp, err := client.Get("https://www.googleapis.com/oauth2/v4/token")
	if err != nil {
		log.Printf(color.RedString("Unable to validate token for %s, removing it and try it again: %s"), mail, err)
		if err := removeAuth(mail); err != nil {
			log.Printf(color.RedString("Unable to delete auth token, exiting: %s"), err)
			panic("Unable to delete auth token")
		}
		return auth(mail)
	} else {
		log.Println(color.CyanString("Authentication successful"))
	}
	defer resp.Body.Close()
	return client
}

func findFolder(s *drive.Service, name string) (*Folder, error) {
	listcall := s.Files.List()
	nextpage := ""
	for {
		if nextpage != "" {
			listcall = listcall.PageToken(nextpage)
		}
		l, err := listcall.Q(fmt.Sprintf("name='%s'", name)).Do()
		if err != nil {
			log.Printf("Unable to list folders: %s", err)
			return nil, err
		}
		for _, item := range l.Files {
			if item.MimeType == "application/vnd.google-apps.folder" && item.Name == name {
				return &Folder{Service: s, ID: item.Id}, nil
			}
		}
		nextpage = l.NextPageToken
		if nextpage == "" {
			break
		}
	}
	return nil, fmt.Errorf("Folder %s not found", name)
}

func getFolders(sourceClient, destClient *drive.Service, source, dest string) (sourceFolder, destFolder *Folder, err error) {
	sourceFolder, err = findFolder(sourceClient, source)
	if err != nil {
		log.Printf("Unable to find folder: %s", err)
		return nil, nil, err
	}

	destFolder, err = findFolder(destClient, dest)
	if err != nil {
		log.Printf("Unable to find folder: %s", err)
		return nil, nil, err
	}

	return sourceFolder, destFolder, nil
}

func driveClient(c *http.Client) (*drive.Service, error) {
	return drive.New(c)
}

func main() {
	flag.Parse()

	ctx = context.Background()
	sourceClient, err := driveClient(auth(*from))
	if err != nil {
		panic(fmt.Sprintf("Unable to create drive client: %s", err))
	}

	destClient, err := driveClient(auth(*to))
	if err != nil {
		panic(fmt.Sprintf("Unable to create drive client: %s", err))
	}

	source, dest, err := getFolders(sourceClient, destClient, *fromFolder, *toFolder)
	if err != nil {
		panic(fmt.Sprintf("Unable to find folders: %s", err))
	}
	source.Mail = *from
	dest.Mail = *to

	log.Printf("Folders: %+v, %+v", source, dest)

	if err := migrateFolder(source, dest); err != nil {
		panic(fmt.Sprintf("Unable to migrate: %s", err))
	}
}
