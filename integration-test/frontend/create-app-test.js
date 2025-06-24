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

  await page.waitForSelector('#apps-button', { visible: true, timeout: 60000  });
  await page.screenshot({ path: 'screenshots/after-click-login.png', fullPage: true });
  await page.click('#apps-button');
  await page.screenshot({ path: 'screenshots/after-click-apps-button.png', fullPage: true });

  
  await page.waitForSelector('#new-app-button', { visible: true, timeout: 60000 });
  await page.click('#new-app-button');


  //  HELIX APP GITHUB AUTHOURIZATION

  // await page.waitForSelector('#connect-github-account', { visible: true });

  // await page.screenshot({ path: 'screenshots/after-click-new-app-button.png', fullPage: true });

  // await page.click('#connect-github-account');
  // await page.waitForNavigation();
  
  // await page.waitForSelector('#login_field', { visible: true });

  // await page.type('#login_field', 'obianuoobinna@gmail.com');

  // await page.waitForSelector('#password', { visible: true });

  // await page.type('#password', process.env.PASSWORD);

  // await page.waitForSelector('#login > div.auth-form-body.mt-3 > form > div > input.btn.btn-primary.btn-block.js-sign-in-button', { visible: true, timeout: 10000 });

  // await page.click('#login > div.auth-form-body.mt-3 > form > div > input.btn.btn-primary.btn-block.js-sign-in-button');
  // await page.waitForNavigation();
  // await page.screenshot({ path: 'screenshots/after-click-github-signin-button-github.png', fullPage: true });

  // await page.waitForSelector('#authorize-button', { visible: true });
  // await page.click('#authorize-button');
  // await page.screenshot({ path: 'screenshots/after-click-authorize-button.png', fullPage: true });

  await page.waitForSelector('#filter-textfield', { visible: true });
  await page.click('#filter-textfield');
  await page.type('#filter-textfield', 'ObianuoObi/example-app-api-template');
  await page.screenshot({ path: 'screenshots/after-typing-filter-textfield.png', fullPage: true });

  await page.waitForSelector('#play-circle-button', { visible: true });
  await page.click('#play-circle-button');
  await page.screenshot({ path: 'screenshots/after-click-play-circle-button.png', fullPage: true });

  await page.waitForSelector('#connect-repo-button', { visible: true });
  await page.click('#connect-repo-button');
  await page.waitForNavigation();
  await page.screenshot({ path: 'screenshots/after-click-connect-repo-button.png', fullPage: true });

  await page.waitForSelector('#cancelButton', { visible: true })
  await page.click('#cancelButton');
  await page.screenshot({ path: 'screenshots/after-click-cancelButton.png', fullPage: true });

  await page.waitForSelector('#home-link', { visible: true })
  await page.click('#home-link');
  await page.screenshot({ path: 'screenshots/after-click-home-link.png', fullPage: true });

  await page.waitForSelector('#home-link', { visible: true })
  await page.click('#home-link');
  await page.screenshot({ path: 'screenshots/after-click-home-link.png', fullPage: true });


  await page.waitForSelector('#browse-button', { visible: true })
  await page.click('#browse-button');
  await page.screenshot({ path: 'screenshots/after-click-browse-button.png', fullPage: true });

  await page.waitForSelector('#launch-button-0', { visible: true })
  await page.click('#launch-button-0');
  await page.screenshot({ path: 'screenshots/after-click-launch-button-0.png', fullPage: true });

  await page.waitForSelector('#textEntry', { visible: true })
  await page.type('#textEntry', 'What is the current price of Bitcoin in yen');
  await page.screenshot({ path: 'screenshots/after-type-text-in-textEntry.png', fullPage: true });


  
  await page.click('#sendButton');
  await page.waitForNavigation();
  await page.screenshot({ path: 'screenshots/after-click-sendButton.png', fullPage: true });


  await browser.close();
})();





















  



  




  


  
  // await page.goto('http://localhost:8080');
  // await bluebird.delay(5000);

  // await page.waitForSelector('.MuiButtonBase-root.MuiButton-root.MuiButton-outlined.MuiButton-outlinedPrimary', { visible: true });
  // await bluebird.delay(5000);
  // await page.click('.MuiButtonBase-root.MuiButton-root.MuiButton-outlined.MuiButton-outlinedPrimary');
  // await page.screenshot({ path: 'screenshots/after-click-login-button.png', fullPage: true });

  // await page.waitForSelector('#username', { visible: true });
  // await page.type('#username', 'helixml.test@gmail.com');
  // await page.screenshot({ path: 'screenshots/after-type-username.png', fullPage: true });

  // await page.type('#password', 'Pa$$-Word');
  // await page.screenshot({ path: 'screenshots/after-type-password.png', fullPage: true });

  // await page.click('#kc-login');
  // await page.screenshot({ path: 'screenshots/after-click-kc-login.png', fullPage: true });


  // await page.waitForSelector('#root-container > main > div > div > div > div > div:nth-child(5) > div > div > div:nth-child(3) > div > div > div > div:nth-child(1) > button', { visible: true, timeout: 10000 });
  // await bluebird.delay(5000);

  // // Click the button using the specific selector
  // await page.click('#root-container > main > div > div > div > div > div:nth-child(5) > div > div > div:nth-child(3) > div > div > div > div:nth-child(1) > button');
  // await page.screenshot({ path: 'screenshots/after-click-specific-button.png', fullPage: true });

  // // Wait for the "New App" button to be visible
  // await page.waitForSelector('#root-container > main > div > div > div.MuiBox-root.css-2uchni > header > div > div > div.MuiBox-root.css-8by7kh > div > button', { visible: true, timeout: 10000 });
  // await bluebird.delay(5000);

  // // Click the "New App" button using the specific selector
  // await page.click('#root-container > main > div > div > div.MuiBox-root.css-2uchni > header > div > div > div.MuiBox-root.css-8by7kh > div > button');
  // await page.screenshot({ path: 'screenshots/after-click-new-app-look.png', fullPage: true });
  // await bluebird.delay(1000);
  // await page.screenshot({ path: 'screenshots/after-click-new-app-button.png', fullPage: true });
  

  // // Wait for the new selector to be visible with increased timeout
  // await page.waitForSelector('body > div.MuiDialog-root.MuiModal-root.css-w9ragz > div.MuiDialog-container.MuiDialog-scrollPaper.css-ekeie0 > div > div.MuiDialogContent-root.css-1ty026z > div > div.MuiBox-root.css-16awsaw > ul > li > div.MuiButtonBase-root.MuiListItem-root.MuiListItem-dense.MuiListItem-gutters.MuiListItem-padding.MuiListItem-button.MuiListItem-secondaryAction.css-1h19asi', { visible: true, timeout: 20000 });
  // await bluebird.delay(5000);

  // // Click the new selector using the specific selector
  // await page.click('body > div.MuiDialog-root.MuiModal-root.css-w9ragz > div.MuiDialog-container.MuiDialog-scrollPaper.css-ekeie0 > div > div.MuiDialogContent-root.css-1ty026z > div > div.MuiBox-root.css-16awsaw > ul > li > div.MuiButtonBase-root.MuiListItem-root.MuiListItem-dense.MuiListItem-gutters.MuiListItem-padding.MuiListItem-button.MuiListItem-secondaryAction.css-1h19asi');
  // await page.screenshot({ path: 'screenshots/after-click-github-connec-button.png', fullPage: true });

  // // Wait for the "Connect Repo" button to be visible with increased timeout
  // await page.waitForSelector('body > div.MuiDialog-root.MuiModal-root.css-w9ragz > div.MuiDialog-container.MuiDialog-scrollPaper.css-ekeie0 > div > div.MuiDialogContent-root.css-1ty026z > div > div.MuiBox-root.css-19midj6 > div > div:nth-child(3) > button.MuiButtonBase-root.MuiButton-root.MuiButton-contained.MuiButton-containedSecondary.MuiButton-sizeSmall.MuiButton-containedSizeSmall.MuiButton-colorSecondary.MuiButton-root.MuiButton-contained.MuiButton-containedSecondary.MuiButton-sizeSmall.MuiButton-containedSizeSmall.MuiButton-colorSecondary.css-1hhg8i9', { visible: true, timeout: 20000 });
  // await bluebird.delay(5000);

  // // Click the "Connect Repo" button using the specific selector
  // await page.click('body > div.MuiDialog-root.MuiModal-root.css-w9ragz > div.MuiDialog-container.MuiDialog-scrollPaper.css-ekeie0 > div > div.MuiDialogContent-root.css-1ty026z > div > div.MuiBox-root.css-19midj6 > div > div:nth-child(3) > button.MuiButtonBase-root.MuiButton-root.MuiButton-contained.MuiButton-containedSecondary.MuiButton-sizeSmall.MuiButton-containedSizeSmall.MuiButton-colorSecondary.MuiButton-root.MuiButton-contained.MuiButton-containedSecondary.MuiButton-sizeSmall.MuiButton-containedSizeSmall.MuiButton-colorSecondary.css-1hhg8i9');
  // await bluebird.delay(5000);
  // await page.screenshot({ path: 'screenshots/after-click-connect-repo-button.png', fullPage: true });

  // await page.click('#root-container > main > div > div > div.MuiBox-root.css-16awsaw > div > div > div > div:nth-child(2) > div.MuiBox-root.css-vycneo > div > div > div > div.InovuaReactDataGrid__body > div.InovuaReactDataGrid__column-layout > div.InovuaReactDataGrid__virtual-list.inovua-react-virtual-list.inovua-react-virtual-list--theme-default-light.inovua-react-virtual-list--virtual-scroll.inovua-react-scroll-container--block.inovua-react-scroll-container.inovua-react-scroll-container--theme-default-light > div > div > div:nth-child(1) > div > div > div.inovua-react-virtual-list__row-container > div > div > div.InovuaReactDataGrid__cell.InovuaReactDataGrid__cell--unlocked.InovuaReactDataGrid__cell--direction-ltr.InovuaReactDataGrid__cell--user-select-text.InovuaReactDataGrid__cell--last.InovuaReactDataGrid__cell--show-border-right.InovuaReactDataGrid__cell--show-border-bottom > div > div > button:nth-child(2) > svg');
  // await page.screenshot({ path: 'screenshots/after-click-copy-api-button.png', fullPage: true });

  // await page.click('#root-container > main > div > div > div.MuiBox-root.css-2uchni > header > div > div > div.MuiBox-root.css-8by7kh > div > button.MuiButtonBase-root.MuiButton-root.MuiButton-contained.MuiButton-containedSecondary.MuiButton-sizeMedium.MuiButton-containedSizeMedium.MuiButton-colorSecondary.MuiButton-root.MuiButton-contained.MuiButton-containedSecondary.MuiButton-sizeMedium.MuiButton-containedSizeMedium.MuiButton-colorSecondary.css-dglpgo');
  // await bluebird.delay(5000);
  // await page.screenshot({ path: 'screenshots/after-click-save-button.png', fullPage: true });

  


  

    // Run the curl command using the API key
  // const curlCommand = `curl -s -i -H "Authorization: Bearer ${apiKey}" https://app.helix.ml/v1/chat/completions --data-raw '{"messages":[{"role":"user","content":"Using the Coinbase API, what is the live Bitcoin price in GBP"}], "model":"llama3:instruct", "stream":false}'`;

  // exec(curlCommand, (error, stdout, stderr) => {
  //   if (error) {
  //     console.error(`Error executing curl command: ${error.message}`);
  //     return;
  //   }
  //   if (stderr) {
  //     console.error(`stderr: ${stderr}`);
  //     return;
  //   }
  //   console.log(`stdout: ${stdout}`);

    

    
  // });

  



  // // Wait for the button in the dialog to be visible
  // await page.waitForSelector('body > div.MuiDialog-root.MuiModal-root.css-w9ragz > div.MuiDialog-container.MuiDialog-scrollPaper.css-ekeie0 > div > div.MuiDialogContent-root.css-1ty026z > div > button', { visible: true, timeout: 10000 });
  // await bluebird.delay(5000);

  // // Click the button in the dialog using the specific selector
  // await page.click('body > div.MuiDialog-root.MuiModal-root.css-w9ragz > div.MuiDialog-container.MuiDialog-scrollPaper.css-ekeie0 > div > div.MuiDialogContent-root.css-1ty026z > div > button');
  // await page.screenshot({ path: 'screenshots/after-click-dialog-button.png', fullPage: true });

  // await page.waitForSelector('#login_field', { visible: true });
  // await page.type('#login_field', 'helixml.test@gmail.com');
  // await page.screenshot({ path: 'screenshots/after-type-username-github.png', fullPage: true });
  
  // await page.waitForSelector('#password', { visible: true });
  // await page.type('#password', 'Pa$$-Word10');
  // await page.screenshot({ path: 'screenshots/after-type-password-github.png', fullPage: true });

  // await page.waitForSelector('#login > div.auth-form-body.mt-3 > form > div > input.btn.btn-primary.btn-block.js-sign-in-button', { visible: true, timeout: 10000 });
  // await bluebird.delay(5000);

  // // Click the GitHub login button using the specific selector
  // await page.click('#login > div.auth-form-body.mt-3 > form > div > input.btn.btn-primary.btn-block.js-sign-in-button');
  // await page.waitForNavigation();
  // await page.screenshot({ path: 'screenshots/after-click-github-login-button-github.png', fullPage: true });

  // await page.waitForSelector('body > div.logged-in.env-production.page-responsive.color-bg-subtle > div.application-main > main > div > div.px-3.mt-5 > div.Box.color-shadow-small > div.Box-footer.p-3.p-md-4.clearfix > div:nth-child(1) > form > div > button.js-oauth-authorize-btn.btn.btn-primary.width-full.ws-normal', { visible: true, timeout: 10000 });
  // await bluebird.delay(5000);

  // // Click the link button using the specific selector
  // await page.click('body > div.logged-in.env-production.page-responsive.color-bg-subtle > div.application-main > main > div > div.px-3.mt-5 > div.Box.color-shadow-small > div.Box-footer.p-3.p-md-4.clearfix > div:nth-child(1) > form > div > button.js-oauth-authorize-btn.btn.btn-primary.width-full.ws-normal');
  // await page.screenshot({ path: 'screenshots/after-click-github-auth-button-github.png', fullPage: true });
  // await bluebird.delay(5000);

  // await page.waitForSelector('body > div.MuiDialog-root.MuiModal-root.css-w9ragz > div.MuiDialog-container.MuiDialog-scrollPaper.css-ekeie0 > div > div.MuiDialogContent-root.css-1ty026z > div > div.MuiBox-root.css-16awsaw > ul > li > div.MuiListItemSecondaryAction-root.css-y3qv5r > span > svg', { visible: true, timeout: 20000 });
  // await bluebird.delay(5000);

  // // Click the new selector using the specific selector
  // await page.click('body > div.MuiDialog-root.MuiModal-root.css-w9ragz > div.MuiDialog-container.MuiDialog-scrollPaper.css-ekeie0 > div > div.MuiDialogContent-root.css-1ty026z > div > div.MuiBox-root.css-16awsaw > ul > li > div.MuiListItemSecondaryAction-root.css-y3qv5r > span > svg');
  // await page.screenshot({ path: 'screenshots/after-click-new-selector.png', fullPage: true });

  // await page.waitForSelector('body > div.MuiDialog-root.MuiModal-root.css-w9ragz > div.MuiDialog-container.MuiDialog-scrollPaper.css-ekeie0 > div > div.MuiDialogContent-root.css-1ty026z > div > div.MuiBox-root.css-16awsaw > ul > li > div.MuiListItemSecondaryAction-root.css-y3qv5r > span > svg', { visible: true, timeout: 20000 });
 

  // // Click the new selector using the specific selector
  // await page.click('body > div.MuiDialog-root.MuiModal-root.css-w9ragz > div.MuiDialog-container.MuiDialog-scrollPaper.css-ekeie0 > div > div.MuiDialogContent-root.css-1ty026z > div > div.MuiBox-root.css-16awsaw > ul > li > div.MuiListItemSecondaryAction-root.css-y3qv5r > span > svg');
  // await page.screenshot({ path: 'screenshots/after-click-new-selector.png', fullPage: true });
  

//   await page.waitForSelector('body > div.MuiDialog-root.MuiModal-root.css-w9ragz > div.MuiDialog-container.MuiDialog-scrollPaper.css-ekeie0 > div > div.MuiDialogContent-root.css-1ty026z > div > div.MuiBox-root.css-16awsaw > ul > li > div.MuiButtonBase-root.MuiListItem-root.MuiListItem-dense.MuiListItem-gutters.MuiListItem-padding.MuiListItem-button.MuiListItem-secondaryAction.css-1h19asi', { visible: true, timeout: 10000 });
//   await bluebird.delay(5000);

// // Click the new selector using the specific selector
//   await page.click('body > div.MuiDialog-root.MuiModal-root.css-w9ragz > div.MuiDialog-container.MuiDialog-scrollPaper.css-ekeie0 > div > div.MuiDialogContent-root.css-1ty026z > div > div.MuiBox-root.css-16awsaw > ul > li > div.MuiButtonBase-root.MuiListItem-root.MuiListItem-dense.MuiListItem-gutters.MuiListItem-padding.MuiListItem-button.MuiListItem-secondaryAction.css-1h19asi');
//   await bluebird.delay(5000);
//   await page.screenshot({ path: 'screenshots/after-click-new-github-app-button.png', fullPage: true });
  

  


  
  // await page.waitForSelector('.MuiIconButton-root', { visible: true, timeout: 10000 });
  // await bluebird.delay(5000);
  // await page.screenshot({ path: 'screenshots/before-click-icon-button.png', fullPage: true });

  // await page.click('.MuiIconButton-root');
  // await page.screenshot({ path: 'screenshots/after-click-icon-button.png', fullPage: true });

  // await page.waitForSelector('.MuiIconButton-root', { visible: true, timeout: 10000 });
  // await bluebird.delay(5000);
  // await page.click('.MuiIconButton-root'); 
  // await page.screenshot({ path: 'screenshots/after-click-icon-button-again.png', fullPage: true });

  // await page.waitForNavigation();
  // await page.waitForSelector('#new-app-button', { visible: true, timeout: 10000 });
  // await bluebird.delay(5000);
  // await page.click('#new-app-button');
  // await page.screenshot({ path: 'screenshots/after-click-new-app-button.png', fullPage: true });

  // await page.hover('#menu-appbar > div.MuiPaper-root.MuiPaper-elevation.MuiPaper-rounded.MuiPaper-elevation8.MuiPopover-paper.MuiMenu-paper.MuiMenu-paper.css-adx198');
  // await bluebird.delay(1000); // Add a small delay to ensure the hover effect takes place


  // await page.evaluate(() => {
  //   const appsMenuItem = document.querySelector('#menu-appbar > div.MuiPaper-root.MuiPaper-elevation.MuiPaper-rounded.MuiPaper-elevation8.MuiPopover-paper.MuiMenu-paper.MuiMenu-paper.css-adx198 > ul > li:nth-child(3)');
  //   if (appsMenuItem) {
  //     appsMenuItem.scrollIntoView();
  //   }
  // });
  // await bluebird.delay(500); 


  //   // Click the "Apps" menu item
  // await page.click('#menu-appbar > div.MuiPaper-root.MuiPaper-elevation.MuiPaper-rounded.MuiPaper-elevation8.MuiPopover-paper.MuiMenu-paper.MuiMenu-paper.css-adx198 > ul > li:nth-child(3)');
  // await page.screenshot({ path: 'screenshots/after-click-apps-menu-item.png', fullPage: true });
