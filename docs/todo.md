## kai 23rd Nov

 - [ ] finish and deploy dashboard
 - [ ] logged out state when trying to do things - show a message "please register"
 - [ ] fix bug with "create image" dropdown etc not working
 - [ ] fix bug with openAI responding with "GPT 4 Answer: Without providing a valid context, I am unable to generate 50 question and answer pairs as requested"
   - [ ] make it so user can see whole message from OpenAI
 - [ ] replace the thinking face with a spinning progress (small horizontal bouncing three dots)
 - [ ] obvious buttons (on fine tuning)
   - [ ] in default starting state - make both buttons (add docs / text) - blue and outlined
   - [ ] the the default starting state - make the files button say "or choose files"
   - [ ] when you start typing in the box make the "Add Text" button pink and make the upload files not pink
   - [ ] once there are > 0 files - make the "choose more files" button outlined so the "upload docs" is the main button
 - [ ] progress bars on text fine tuning
 - [ ] performance on text fine tuning
 - [ ] pdfs are broken in production
 - [ ] hide technical details behind "technical details" button ?
   - [ ] where it currently says "Session ...." - put the session title
   - [ ] put a link next to "View Files" called "Info" that will open a model window with more session details
   - [ ] e.g. we put the text summary above in the model along with the ID and other things we want to show
   - [ ] in the text box say "Chat with Helix" <- for txt models
 - [ ] react is rendering streaming updates to the sessions slowly
 - [ ] URL to fetch text for text fine tuning
 - [ ] edit session name (pencil icon to left of bin icon)
 - [ ] retry button for errors
 - [ ] fork session (fork from an interaction)
 - [ ] multiple SDXL at the same time causes error
   - [ ] this could be in same session or not
   - [ ] we need a test stack
 - [ ] place in the queue indiciation if it's more than 5 seconds
 - [x] homepage uncomment buttons
 - [ ] reliable and fast, scale to 5 concurrent users (Luke)
   - [ ] Dockerize the runner & deploy some on vast.ai / runpod.io
 - [ ] show API calls to replicate many actions (e.g. text & image inference to start with)
 - [ ] the data prep needs to be in a job queue (later, unless we do everything ourselves)
   - [ ] the same job queue would be used for fetching data from URLs

 