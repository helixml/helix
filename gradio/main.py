import gradio as gr

def cowsay(message):
    return "Hello " + message + "!"

demo = gr.Interface(
    fn=cowsay,
    inputs=gr.Textbox(lines=2, placeholder="What would you like the cow to say?"),
    outputs="text",
    allow_flagging="never"
)
demo.launch(server_name="0.0.0.0",
            root_path="/gradio")
