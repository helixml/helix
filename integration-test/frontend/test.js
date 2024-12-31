import puppeteer from 'puppeteer'


(async () => {
  const browser = await puppeteer.launch({headless: false });
  const page = await browser.newPage();
  await page.setViewport({ width: 1280, height: 800 });


  await page.goto('http://localhost:8080');
 

  await page.waitForSelector('#login-button', { visible: true });
  await page.click('#login-button');
  await page.screenshot({ path: 'screenshots/after-click-login-button.png', fullPage: true });
 
  await page.waitForSelector('#username', { visible: true });
  await page.type('#username', 'obianuoobinna@gmail.com');
  await page.type('#password', 'password');
  await page.screenshot({ path: 'screenshots/after-type-username.png', fullPage: true });
  await page.click('#kc-login');
  await page.screenshot({ path: 'screenshots/after-click-kc-login.png', fullPage: true });

  
   await page.waitForSelector('#upload-button', { visible: true, timeout: 5000 });
   await page.click('#upload-button');
   await page.screenshot({ path: 'screenshots/after-click-upload.png', fullPage: true });


   await page.waitForSelector('#manual-text-file-input', { visible: true });
   await page.type('#manual-text-file-input', 'The phone number of Bob is 3813308004');
   await page.screenshot({ path: 'screenshots/after-type-manual-text-file-input.png', fullPage: true });


  //  await page.click('.MuiButtonBase-root.MuiIconButton-root.MuiIconButton-sizeMedium.css-1udjy19-MuiButtonBase-root-MuiIconButton-root');
  //  await page.screenshot({ path: 'screenshots/after-click-url-input-button.png', fullPage: true });

  //  await page.waitForSelector('#root-container > main > div > div > div:nth-child(3) > div > div > div:nth-child(3) > button');
  //  await page.click('#root-container > main > div > div > div:nth-child(3) > div > div > div:nth-child(3) > button');
  //  await page.screenshot({ path: 'screenshots/after-click-continued-button.png', fullPage: true });
  //  await page.reload({ waitUntil: ['networkidle0', 'domcontentloaded'] });
  //  await page.reload({ waitUntil: ['networkidle0', 'domcontentloaded'] });
  //  await page.reload({ waitUntil: ['networkidle0', 'domcontentloaded'] });
   
   


  //  await page.screenshot({ path: 'screenshots/after-page-reload.png', fullPage: true });

  //  await page.waitForSelector('#textEntry', { visible: true, timeout: 50000  });
  //  await page.type('#textEntry', 'select all the icons that start with the letter o');
  //  await page.screenshot({ path: 'screenshots/after-input-textEntry.png', fullPage: true });
 
   
   
 
  //  await page.screenshot({ path: 'screenshots/before-click-continue-button.png', fullPage: true });

  //  await page.click('.MuiButtonBase-root.MuiButton-root.MuiButton-contained.MuiButton-containedPrimary.MuiButton-sizeMedium.MuiButton-containedSizeMedium.MuiButton-colorPrimary.MuiButton-root.MuiButton-contained.MuiButton-containedPrimary.MuiButton-sizeMedium.MuiButton-containedSizeMedium.MuiButton-colorPrimary.css-gb3rsc-MuiButtonBase-root-MuiButton-root');
 
  // //  await page.click('#continue-button');
 
  //  await delay(5000);
 
  //  await page.screenshot({ path: 'screenshots/after-click-continue-button.png', fullPage: true });
 
  //  // Wait for the text entry to be visible
  //  await page.waitForSelector('#textEntry', { visible: true, timeout: 10000 });
  //  await delay(5000);
 
  //  await page.screenshot({ path: 'screenshots/before-type-textEntry.png', fullPage: true });
 
  //  await page.type('#textEntry', 'select all the icons that start with the letter o');
  //  await delay(5000);

  //  await page.screenshot({ path: 'screenshots/after-type-textEntry.png', fullPage: true });

  //  await page.click('#send-button');

  //  await delay(5000);

  //  await page.screenshot({ path: 'screenshots/after-click-sendButton.png', fullPage: true });

  //  await delay(5000);

  //  await page.screenshot({ path: 'screenshots/after-log-console.png', fullPage: true });

   await browser.close();
})();

 