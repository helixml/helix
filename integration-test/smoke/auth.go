package main

import (
	"fmt"
	"os"

	"github.com/go-rod/rod"
)

func performLogin(browser *rod.Browser) error {
	page := browser.MustPage(getServerURL())
	page.MustWaitLoad()

	cookieStore := NewCookieStore("")
	if err := cookieStore.Load(page, getServerURL()); err != nil {
		return loginWithCredentials(page)
	}

	return verifyLogin(page)
}

func loginWithCredentials(page *rod.Page) error {
	logStep("Looking for login button")
	loginBtn := page.MustElement("button[id='login-button']")
	loginBtn.MustClick()

	logStep("Waiting for login page to load")
	page.MustWaitLoad()

	logStep("Getting credentials from environment")
	username := getHelixUser()
	password := getHelixPassword()

	logStep("Filling in username and password")
	page.MustElement("input[type='text']").MustInput(username)
	page.MustElement("input[type='password']").MustInput(password)
	page.MustElement("input[type='submit']").MustClick()
	page.MustWaitStable()

	cookieStore := NewCookieStore("")
	return cookieStore.Save(page, getServerURL())
}

func verifyLogin(page *rod.Page) error {
	logStep("Waiting for page to stabilize")
	page.MustWaitStable()

	logStep("Verifying login")
	username := os.Getenv("HELIX_USER")
	xpath := fmt.Sprintf(`//span[contains(text(), '%s')]`, username)
	if len(page.MustElementsX(xpath)) == 0 {
		return fmt.Errorf("login failed - username not found")
	}
	return nil
}
