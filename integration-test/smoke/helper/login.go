package helper

import (
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/go-rod/rod"
)

func ClickLoginButton(t *testing.T, page *rod.Page) {
	loginButtonSelector := "button[id='login-button']"
	LogStep(t, fmt.Sprintf("Waiting for login button ('%s') to appear...", loginButtonSelector))

	waitErr := page.Wait(rod.Eval(`(selector) => document.querySelector(selector) !== null`, loginButtonSelector))
	if waitErr != nil {
		t.Fatalf("Timed out waiting for login button ('%s') to appear: %v", loginButtonSelector, waitErr)
	}

	LogStep(t, "Login button found. Clicking via JS (bypassing stability check)")

	page.MustEval(`(selector) => {
		const btn = document.querySelector(selector);
		if (!btn) {
			throw new Error('Login button (' + selector + ') found by Wait but not by Eval');
		}
		btn.click();
	}`, loginButtonSelector)
}

func GetHelixUser() string {
	user := os.Getenv("HELIX_USER")
	if user == "" {
		log.Fatal("HELIX_USER environment variable is not set")
	}
	return user
}

func GetHelixPassword() string {
	password := os.Getenv("HELIX_PASSWORD")
	if password == "" {
		log.Fatal("HELIX_PASSWORD environment variable is not set")
	}
	return password
}

func PerformLogin(t *testing.T, page *rod.Page) error {
	if err := loginWithCredentials(t, page); err != nil {
		return err
	}
	return verifyLogin(t, page)
}

func loginWithCredentials(t *testing.T, page *rod.Page) error {
	ClickLoginButton(t, page)

	LogStep(t, "Getting credentials from environment")
	username := GetHelixUser()
	password := GetHelixPassword()

	LogStep(t, "Filling in username and password")
	page.MustElementX("//input[@type='text']").MustWaitVisible().MustInput(username)
	page.MustElementX("//input[@type='password']").MustWaitVisible().MustInput(password)
	page.MustElementX("//input[@type='submit']").MustWaitVisible().MustClick()

	return nil
}

func verifyLogin(t *testing.T, page *rod.Page) error {
	LogStep(t, "Verifying login")
	username := GetHelixUser()
	xpath := fmt.Sprintf(`//span[contains(text(), '%s')]`, username)
	el := page.MustElementX(xpath)
	if el == nil {
		return fmt.Errorf("login failed - username not found")
	}
	return nil
}

func RegisterNewUser(t *testing.T, page *rod.Page) {
	ClickLoginButton(t, page)
	LogStep(t, "Looking for register button")
	page.MustElementX("//a[contains(text(), 'Register')]").MustWaitVisible().MustClick()

	LogStep(t, "Filling in username and password")
	page.MustElementX("//input[@id='firstName']").MustWaitVisible().MustInput("test")
	page.MustElementX("//input[@id='lastName']").MustWaitVisible().MustInput("test")
	page.MustElementX("//input[@id='email']").MustWaitVisible().MustInput("test@test.com")
	page.MustElementX("//input[@id='password']").MustWaitVisible().MustInput("test")
	page.MustElementX("//input[@id='password-confirm']").MustWaitVisible().MustInput("test")
	page.MustElementX("//input[@type='submit']").MustWaitVisible().MustClick()

	LogStep(t, "Verifying registration")
	xpath := fmt.Sprintf(`//span[contains(text(), '%s')]`, "test@test.com")
	page.MustElementX(xpath).MustVisible()
}
