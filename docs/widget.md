# widget

The widget allows users to paste a script tag into their website that will render a UI for a chat session.

It works using the https://github.com/helixml/chat-widget library.

We have an additional vite build config (`vite-widget.config.ts`), which is used to build the widget.  It will bundle using the [IIFE](https://en.wikipedia.org/wiki/Immediately_invoked_function_expression) format which means that we can put this script tag on any page that uses any JS library - it is totally self contained and non combative with other versions of React or other libraries the user might have installed.

## configuring the embedded widget

We have an api endpoint that will accept query parameters and turn them into configuration for the widget JS.

This means that users can configure the widget by changing the query parameters in the script tag, this opens a world of useful tooling (e.g. a UI for designing themes for the widget).

To know what query parameters are available, you can look at the config https://github.com/helixml/chat-widget

Any theme property can by splitting the theme name and property using a `.` e.g. `searchTheme.backgroundColor=red`

For example, to embed a widget with the url `https://app.tryhelix.ai/api/v1/completions` and model of `mistral:7b-instruct` with a red background for the text field:

```html
<script src="http://localhost/api/v1/widget?url=https://app.tryhelix.ai/api/v1/completions&model=mistral:7b-instruct&searchTheme.backgroundColor=red"></script>
```

## widget javascript

The following command will build the widget JS file:

```bash
cd frontend
yarn build-widget
```

This will produce `frontend/dist/helix-embed.iife.js` - which is then loaded by the api when it responds to the widget script tag request.

The api is configured with where to load this file from using the `WIDGET_JS_PATH` environment variable.

## development

Because the widget is bundled using a second vite config - we cannot serve it's build output using the normal vite dev server.  Instead we manually build the widget and copy the file into the api container.

The development docker-compose file will point at `/tmp/helix-embed.iife.js` for the widget JS file.

Here is the command to get the widget file into the api container for the development stack

```bash
(cd frontend && yarn build-widget)
docker cp ./frontend/dist/helix-embed.iife.js helix_api_1:/tmp/helix-embed.iife.js
```

## production

In productio, the widget file is built and output into the `dist` folder alongside the rest of the frontend build.

The production docker-compose file will point at `/www/helix-embed.iife.js` for the widget JS file.
