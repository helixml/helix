import puppeteer from 'puppeteer'


(async () => {
  const browser = await puppeteer.launch({headless: false });
  const page = await browser.newPage();
  await page.setViewport({ width: 1280, height: 800 });

  await page.goto('http://localhost:8080/');
 

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

  await page.waitForSelector('#text-finetune-button', { visible: true, timeout: 60000  });
  await page.click('#text-finetune-button');
  await page.screenshot({ path: 'screenshots/after-click-text-finetune-button.png', fullPage: true });

  await page.waitForSelector('#url-input', { visible: true, timeout: 60000  });
  await page.click('#url-input');
  await page.type('#url-input', 'https://arxiv.org/pdf/2408.03968');
  await page.screenshot({ path: 'screenshots/after-type-text-in-url1-input.png', fullPage: true });
  await page.click('#add-icon-button');
  await page.screenshot({ path: 'screenshots/after-click-add-icon1-button.png', fullPage: true });

  await page.waitForSelector('#url-input', { visible: true, timeout: 60000  });
  await page.click('#url-input');
  await page.type('#url-input', 'https://arxiv.org/pdf/2408.04082');
  await page.screenshot({ path: 'screenshots/after-type-text-in-url2-input.png', fullPage: true });
  await page.click('#add-icon-button');
  await page.screenshot({ path: 'screenshots/after-click-add-icon2-button.png', fullPage: true });

  await page.waitForSelector('#root-container > main > div > div > div:nth-child(3) > div > div > div:nth-child(3) > button');
  await page.click('#root-container > main > div > div > div:nth-child(3) > div > div > div:nth-child(3) > button');
  // await page.waitForNavigation({ waitUntil: 'networkidle0' }).catch(() => {});
  await page.waitForNetworkIdle({ timeout: 60000, idleTime: 500 }).catch(() => {});
  await page.screenshot({ path: 'screenshots/after-click-continue-button.png', fullPage: true });

  
  

  

  await browser.close();
})();
