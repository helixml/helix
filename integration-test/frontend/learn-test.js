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

  await page.waitForNavigation()

  await page.waitForSelector('#new-session-link', { visible: true });
  await page.click('#new-session-link');
  await page.screenshot({ path: 'screenshots/after-click-new-session-link.png', fullPage: true });

  await page.waitForSelector('#learn-mode', { visible: true, timeout: 60000 });
  await page.click('#learn-mode');
  await page.screenshot({ path: 'screenshots/after-click-learn-mode.png', fullPage: true });

  await page.waitForSelector('#text-input', { visible: true });
  await page.click('#text-input');
  await page.type('#text-input', 'The phone number of Bob is 3813308004');
  await page.screenshot({ path: 'screenshots/after-typing-text-input.png', fullPage: true });

  await page.waitForSelector('#text-add-icon-button', { visible: true });
  await page.click('#text-add-icon-button');
  await page.screenshot({ path: 'screenshots/after-click-text-add-icon-button.png', fullPage: true });

  await page.waitForSelector('#root-container > main > div > div > div:nth-child(3) > div > div > div:nth-child(3) > button');
  await page.click('#root-container > main > div > div > div:nth-child(3) > div > div > div:nth-child(3) > button');
  await page.screenshot({ path: 'screenshots/after-click-continuedd-button.png', fullPage: true });

  await page.waitForSelector('#textEntry', { visible: true });
  await page.click('#textEntry');
  await page.type('#textEntry', 'Does the result contain that number');
  await page.screenshot({ path: 'screenshots/after-typing-text-input.png', fullPage: true });

  await page.waitForSelector('#send-button', { visible: true });
  await page.click('#send-button');
  await page.reload({ waitUntil: ['networkidle0', 'domcontentloaded'] });
  await page.screenshot({ path: 'screenshots/after-click-send-button.png', fullPage: true });


  await browser.close();
})();
