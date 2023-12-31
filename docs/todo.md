## kai

 
 - [x] when continuing a cloned session, the messages are missing
 - [x] if there are no files - the "view files" button shows an error

 - [x] "add new documents" button at bottom of text session (add more documents, dataprep new ones into jsonl qa-pairs, concatenate qa-pairs, retrain model)
 - [x] retry button for errors
 - [ ] render markdown
 - [x] plugin sentry
 - [x] share mode where original training data is not copied
 - [ ] the delete button shows for read only folders in filestore
 - [x] auto-scroll broken
 - [ ] reverse the color of the active session
 - [ ] ensure the order of things in the dashboard
 - [ ] some URLs are just javascript and break unstructured - we need a better error: https://www.reuters.com/legal/colorado-ballot-case-adds-fuel-trumps-nomination-drive-2023-12-20/
 - [x] put the name of the session in topbar
 - [x] rather than system as the username, put the name of the session
 - [ ] sessions are updating other sessions https://mlops-community.slack.com/archives/C0675EX9V2Q/p1702476943225859
 - [ ] why do finetunes stick around in GPU memory? once they're done they should exit right?
 - [ ] make it clear that URLs need to be of text content - for example a youtube URL will not work
 - [ ] detect when we did not manage to extract any text and tell the user that is the error
 - [x] add a restart button whilst doing a fine-tune so if things get stuck we can restart
   - [x] possibly only show this if we've not seen any progress for > 30 seconds (fixed by the error throwing an error if runner reports job still active)
 - [ ] the session page scrolls to the bottom randomly
 - [ ] quite often there's a model ready to serve and a new one gets spun up on the other node - maybe the clocks are drifting between the machines so the 2 second head start doesn't work? or the python processes aren't polling every 100ms or something?
 - [ ] speedway empty error https://mlops-community.slack.com/archives/C0675EX9V2Q/p1701725991656799
 - [ ] empty response messages error https://mlops-community.slack.com/archives/C0675EX9V2Q/p1701727773319809
 - [x] dashboard not showing finetune interactions
 - [ ] we are getting nignx 500's in the runner "load session from api" handler https://mlops-community.slack.com/archives/C0675EX9V2Q/p1702369315736539
 - [x] performance of auto-save before login (image fine tune text is slow)
 - [ ] autoscale spot runpod instances to match our queue depth: https://graphql-spec.runpod.io/ https://docs.runpod.io/recipes/ 
 - [ ] for session updates check we are on the same page
   - [ ] whilst we are on one page and another session is processing - it's updating the page we are on with the wrong session
 - [x] react is rendering streaming updates to the sessions slowly
 - [ ] if you put a URL to a file in the URL box - detect the bloody mime type so we don't split docs that are downloaded
   - [ ] the URL box should download files first
 - [ ] when pasting the link into text fine tuning https://techycompany.com/ the text file has no name
 - [ ] can you make it use GiB not GB? as in, gibibytes 1GB = 1024 * 1024 * 1024 bytes
 - [ ] timestamps on the log events for runner scheduling decisions
 - [ ] show a dot next to sessions that are currently active or have new replies
 - [x] progress bars on text fine tuning
 - [x] fork session (fork from an interaction)
 - [x] add data after the model is trained
 - [x] pdfs are broken in production
 - [x] for HTML conversion, use pupetteer to render the page into a PDF then convert the PDF into plain text
 - [ ] multiple SDXL at the same time causes error
   - [ ] this could be in same session or not
   - [ ] we need a test stack
 - [ ] place in the queue indiciation if it's more than 5 seconds
 - [ ] multi-model group (i.e. train an image and text and combine them into one chat)
 - [ ] add your own runner
 - [x] reliable and fast, scale to 5 concurrent users (Luke)
   - [x] Dockerize the runner & deploy some on vast.ai / runpod.io
 - [ ] show API calls to replicate many actions (e.g. text & image inference to start with)
 - [ ] kill any pid that shows up in nvidia-smi that it doesn't own
 - [ ] email user when their finetune completes
 - [ ] auto-training - i.e. on a schedule, retrain the model from existing connections
 - [ ] the data prep needs to be in a job queue (later, unless we do everything ourselves)
   - [ ] the same job queue would be used for fetching data from URLs


 - [x] finish and deploy dashboard
 - [x] logged out state when trying to do things - show a message "please register"
 - [x] fix bug with "create image" dropdown etc not working
 - [x] fix bug with openAI responding with "GPT 4 Answer: Without providing a valid context, I am unable to generate 50 question and answer pairs as requested"
   - [x] make it so user can see whole message from OpenAI
 - [x] replace the thinking face with a spinning progress (small horizontal bouncing three dots)
 - [x] there is a dashboard bug where where runner model job history reverses itself
 - [x] you lose keyboard focus when the chat box disables and re-enables
 - [x] make the chatbox have keyboard focus the first time you load the page
 - [x] pasting a long chunk of text into training text box makes the box go taller than the screen and you cannot scroll
 - [x] create images says “chat with helix” should say “describe what you want to see in an image”
 - [x] enforce min-width on left sidebar
 - [x] the event cancel handler on drop downs is not letting you click the same mode
 - [x] hide technical details behind "technical details" button ?
   - [x] where it currently says "Session ...." - put the session title
   - [x] put a link next to "View Files" called "Info" that will open a model window with more session details
   - [x] e.g. we put the text summary above in the model along with the ID and other things we want to show
   - [x] in the text box say "Chat with Helix" <- for txt models
   - [x] in the text box say "Make images with Helix" <- for image models
 - [x] edit session name (pencil icon to left of bin icon)
 - [x] obvious buttons (on fine tuning)
   - [x] in default starting state - make both buttons (add docs / text) - blue and outlined
   - [x] the the default starting state - make the files button say "or choose files"
   - [x] when you start typing in the box make the "Add Text" button pink and make the upload files not pink
   - [x] once there are > 0 files - make the "choose more files" button outlined so the "upload docs" is the main button
 - [x] performance on text fine tuning (add concurrency to openAI calls)
 - [x] URL to fetch text for text fine tuning
 - [x] homepage uncomment buttons
 

## bots, sharing and editing

 - [ ] server and controller handlers for create / update bot
 - [ ] page / controller for list bots
 - [ ] show bot at top of session
 - [ ] form fields for description, icon and pre-prompt
 - [x] re-train, will add more interactions to add files to
 - [x] we should keep previous Lora files at the interaction level
 - [x] we hoist lora_dir from the latest interaction to the session
 - [ ] /bot/XXX page that will spawn new session
 - [ ] share and when shared, hide the files (i.e. publish a bot where it's just inference)
 - [ ] share as well as create bot
   - [ ] readonly view
   - [ ] ready to spawn own chat from that place
 
 