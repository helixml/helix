"""
How to launch your Gradio app on a custom path, in this case localhost:8000/gradio

Run this from the terminal as you would normally start a FastAPI app: `uvicorn run:app`
and navigate to http://localhost:8000/gradio in your browser.
"""
from fastapi import FastAPI
import gradio as gr

CUSTOM_PATH = "/gradio"

app = FastAPI()

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
