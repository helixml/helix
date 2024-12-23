package main

import (
	"fmt"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func performLogin(page *rod.Page) error {
	if err := loginWithCredentials(page); err != nil {
		return err
	}
	return verifyLogin(page)
}

func loginWithCredentials(page *rod.Page) error {
	logStep("Looking for login button")
	loginBtn := page.MustElement("button[id='login-button']")
	err := loginBtn.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		logStep("Login button not found, must be already logged in")
		return nil
	}

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

	return nil
}

func verifyLogin(page *rod.Page) error {
	logStep("Verifying login")
	username := getHelixUser()
	xpath := fmt.Sprintf(`//span[contains(text(), '%s')]`, username)
	el := page.MustElementX(xpath)
	if el == nil {
		return fmt.Errorf("login failed - username not found")
	}
	return nil
}
