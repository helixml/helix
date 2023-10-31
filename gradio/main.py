from fastapi import FastAPI
import gradio as gr

# TODO: implement multiple pages within the app as separate gradio apps within
# this python process

# must match path nginx/noxy is proxying to (see docker-compose.yml)
CUSTOM_PATH = "/gradio"

app = FastAPI()

# should never access this route directly
@app.get("/")
def read_main():
    return {"message": "here be dragons"}

def cowsay(message, request: gr.Request):
    return "Hello " + message + "! " + str(dict(request.query_params))

def alternatingly_agree(message, history):
    if len(history) % 2 == 0:
        return f"Yes, I do think that '{message}'"
    else:
        return "I don't think so"

# TODO: show the API call made to LilySaaS API in the UI, so users can see
# easily how to recreate it

APPS = {
    "cowsay":
        gr.Interface(
            fn=cowsay,
            inputs=gr.Textbox(lines=2, placeholder="What would you like the cow to say?"),
            outputs="text",
            allow_flagging="never",
            css="footer {visibility: hidden}"
        ),
    "sdxl":
        gr.Interface(
            fn=cowsay,
            inputs=gr.Textbox(lines=2, placeholder="Enter prompt for SDXL"),
            outputs="image",
            allow_flagging="never",
            css="footer {visibility: hidden}"
        ),
    "mistral7b":
        gr.ChatInterface(alternatingly_agree,
            css="footer {visibility: hidden}"
        ),
}

for (app_name, gradio_app) in APPS.items():
    print("mounting app", app_name, "->", gradio_app)
    app.mount(CUSTOM_PATH+"/"+app_name, gr.routes.App.create_app(gradio_app))