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

io = gr.Interface(
    fn=cowsay,
    inputs=gr.Textbox(lines=2, placeholder="What would you like the cow to say?"),
    outputs="text",
    allow_flagging="never"
)

gradio_app = gr.routes.App.create_app(io)

app.mount(CUSTOM_PATH, gradio_app)