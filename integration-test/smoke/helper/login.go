package helper

import (
	"fmt"
	"testing"

	"github.com/go-rod/rod"
)

func ClickLoginButton(t *testing.T, page *rod.Page) {
	LogStep(t, "Looking for login button")
	page.MustElement("button[id='login-button']").MustClick()
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
