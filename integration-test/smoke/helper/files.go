package helper

import (
	"testing"

	"github.com/go-rod/rod"
)

func BrowseToFilesPage(t *testing.T, page *rod.Page) {
	LogStep(t, "Browsing to the Files page")
	page.MustElement("button[aria-controls='menu-appbar']").MustClick()
	page.MustElementX(`//li[contains(text(), 'Files')]`).MustClick()
}

func CreateFolder(t *testing.T, page *rod.Page, folderName string) {
	LogStep(t, "Creating a new folder")
	page.MustElementX(`//button[text() = 'Create Folder']`).MustClick()
	page.MustElement(`input[type=text]`).MustInput(folderName)
	page.MustElementX(`//button[text() = 'Save']`).MustClick()
}

func BrowseToFolder(t *testing.T, page *rod.Page, folderName string) {
	LogStep(t, "Browsing to the folder")
	page.MustElementX(`//a[contains(text(), '` + folderName + `')]`).MustClick()
}

func UploadFile(t *testing.T, page *rod.Page, filePath string) {
	LogStep(t, "Uploading a file")
	upload := page.MustElement("input[type='file']")

	LogStep(t, "Uploading the file")
	wait1 := page.MustWaitRequestIdle()
	upload.MustSetFiles(filePath)
	wait1()
}
