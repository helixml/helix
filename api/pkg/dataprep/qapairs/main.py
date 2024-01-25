import requests
import json

def load_file(file_path):
    with open(file_path, 'r') as file:
        return file.read()

def chat_with_model(file_content):
    url = "http://mind.local:11434/api/chat"
    headers = {"Content-Type": "application/json"}
    messages = [{"role": "system", "content": "You are an intelligent professor. You create question and answer pairs from given context for your students. Respond with an array of strict JSON 'question' & 'answer' pairs."},
                {"role": "user", "content": f"Here is your context:\n{file_content}"}]
    data = {
        "model": "nous-hermes2-mixtral",
        "messages": messages,
        "stream": False
    }
    print(f"Raw request:")
    import pprint; pprint.pprint(data)
    response = requests.post(url, headers=headers, data=json.dumps(data))
    body = response.json()
    print("Raw response:")
    import pprint; pprint.pprint(body)

file_content = load_file('text.txt')
response = chat_with_model(file_content)
# for message in response:
#     print(message['message']['content'])