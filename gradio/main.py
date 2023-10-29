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

def cowsay(message):
    return "Hello " + message + "!"

def alternatingly_agree(message, history):
    if len(history) % 2 == 0:
        return f"Yes, I do think that '{message}'"
    else:
        return "I don't think so"

# TODO: update the following to call different functions which call into lilypad
io = gr.TabbedInterface([
        gr.Interface(
            fn=cowsay,
            inputs=gr.Textbox(lines=2, placeholder="Enter prompt for SDXL"),
            outputs="image",
            allow_flagging="never"
        ),
        gr.ChatInterface(alternatingly_agree),
        gr.Interface(
            fn=cowsay,
            inputs=gr.Textbox(lines=2, placeholder="What would you like the cow to say?"),
            outputs="text",
            allow_flagging="never"
        ),
        ], ["Stable Diffusion XL", "Talk to Mistral", "Cowsay"],
        css="footer {visibility: hidden}"
)

gradio_app = gr.routes.App.create_app(io)

app.mount(CUSTOM_PATH, gradio_app)