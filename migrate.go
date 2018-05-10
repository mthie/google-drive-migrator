package main

import (
	"fmt"
	"log"

	"github.com/fatih/color"
)

func migrateFolder(src, dst *Folder) (err error) {
	listcall := src.Service.Files.List().Fields("files(id,name,ownedByMe,owners)")
	nextpage := ""
	for {
		if nextpage != "" {
			listcall = listcall.PageToken(nextpage)
		}
		parentSearch := fmt.Sprintf("'%s' in parents", src.ID)
		log.Printf("Search parent: %s", parentSearch)
		l, err := listcall.Q(parentSearch).Do()
		if err != nil {
			log.Printf("Unable to list folders: %s", err)
			return err
		}
		// log.Printf("Result: %+v, %+v", l, l.Files)
		for _, item := range l.Files {
			if item.MimeType == "application/vnd.google-apps.folder" {
				log.Printf("///// Migrate folder %s (%+v) ////////", item.Name, item.Id)
				continue
			}
			//log.Printf("Now migrate file: %+v", item)
			log.Printf(color.GreenString("File: %s, OwnedByMe: %+v"), item.Name, item.OwnedByMe)
			for _, owner := range item.Owners {
				log.Printf(color.YellowString("Owner: %+v"), owner)
			}
			// log.Printf("File: %+v", item)
			nextPermissionPage := ""
			permissionsCall := src.Service.Permissions.List(item.Id).Fields("permissions(kind,id,type,emailAddress,domain,role,allowFileDiscovery,displayName,photoLink,expirationTime,teamDrivePermissionDetails,deleted)")
			for {
				if nextPermissionPage != "" {
					permissionsCall = permissionsCall.PageToken(nextPermissionPage)
				}
				permissions, err := permissionsCall.Do()
				if err != nil {
					log.Printf(color.RedString("Unable to retrieve permissions: %s"), err)
					continue
				}
				for _, p := range permissions.Permissions {
					log.Printf("Single permission: %+v", p)
				}
				log.Printf("Permissions for the file: %+v", permissions)
				nextPermissionPage = permissions.NextPageToken
				if nextPermissionPage == "" {
					break
				}
			}
		}
		nextpage = l.NextPageToken
		if nextpage == "" {
			break
		}
	}
	log.Printf("Folder migrated")

	return nil
}
