import puppeteer from 'puppeteer'


(async () => {
  const browser = await puppeteer.launch({headless: false });
  const page = await browser.newPage();
  await page.setViewport({ width: 1280, height: 800 });

  await page.goto('http://app.helix.ml');
 

  await page.waitForSelector('#login-button', { visible: true });
  await page.click('#login-button');
  await page.screenshot({ path: 'screenshots/after-click-login-button.png', fullPage: true });
 
  await page.waitForSelector('#username', { visible: true });
  await page.type('#username', 'obianuoobinna@gmail.com');
  await page.screenshot({ path: 'screenshots/after-type-username.png', fullPage: true });
 
  await page.type('#password', 'password');
  await page.screenshot({ path: 'screenshots/after-type-username.png', fullPage: true });

  await page.click('#kc-login');
  await page.screenshot({ path: 'screenshots/after-click-login.png', fullPage: true });

  await page.waitForNavigation()

  await page.waitForSelector('#create-button', { visible: true, timeout: 60000  });
  await page.click('#create-button');
  await page.screenshot({ path: 'screenshots/after-click-create-button.png', fullPage: true });


  await page.waitForSelector('#textEntry', { visible: true });
  await page.click('#textEntry');
  await page.type('#textEntry', 'create an image for an interior design for a [adjective describing luxury] master bedroom, featuring [materials] furniture, [style keywords]');
  await page.screenshot({ path: 'screenshots/after-typing-text-input.png', fullPage: true });

  await page.click('#sendButton');
  await page.waitForNetworkIdle().catch(() => {}); // works if page takes time to load
  await page.screenshot({ path: 'screenshots/after-click-sendButton.png', fullPage: true });



  await browser.close();
})();

