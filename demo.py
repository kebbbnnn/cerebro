import os
import sys
import urllib.request
import json
import time

# Custom opener that sets User-Agent
class CustomHTTPRedirectHandler(urllib.request.HTTPRedirectHandler):
    pass

def make_request(url, data=None, headers=None, method="GET"):
    if headers is None:
        headers = {}
    headers["User-Agent"] = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
    
    encoded_data = json.dumps(data).encode("utf-8") if data is not None else None
    req = urllib.request.Request(url, data=encoded_data, headers=headers, method=method)
    return req

def test_chat_completion():
    print("--- 1. Testing Chat Completion via Cerebro Proxy ---")
    url = "http://localhost:8080/v1/chat/completions"
    headers = {
        "Content-Type": "application/json",
        "Authorization": "Bearer cbr-change-me-personal"
    }
    
    models = get_models()
    if not models:
        print("Could not retrieve models list.")
        return
    
    selected_model = models[0]
    print(f"Using model: {selected_model}")
    
    data = {
        "model": selected_model,
        "messages": [{"role": "user", "content": "Tell me a very short 1-sentence joke."}],
        "stream": True
    }
    
    req = make_request(url, data=data, headers=headers, method="POST")
    try:
        with urllib.request.urlopen(req) as response:
            print("Response stream started:")
            for line in response:
                line_str = line.decode("utf-8").strip()
                if not line_str:
                    continue
                if line_str.startswith("data:"):
                    data_content = line_str[5:].strip()
                    if data_content == "[DONE]":
                        break
                    try:
                        chunk = json.loads(data_content)
                        delta = chunk["choices"][0]["delta"]
                        if "content" in delta:
                            print(delta["content"], end="", flush=True)
                    except Exception:
                        pass
            print("\nStream finished successfully.")
    except Exception as e:
        print(f"\nError testing chat completion: {e}")

def get_models():
    url = "http://localhost:8080/v1/models"
    headers = {
        "Authorization": "Bearer cbr-change-me-personal"
    }
    req = make_request(url, headers=headers)
    try:
        with urllib.request.urlopen(req) as response:
            res = json.loads(response.read().decode("utf-8"))
            models = [m["id"] for m in res.get("data", [])]
            return models
    except Exception as e:
        print(f"Error fetching models: {e}")
        return []

def print_stats():
    print("\n--- 2. Fetching Stats ---")
    url = "http://localhost:8080/stats"
    headers = {
        "Authorization": "Bearer cbr-change-me-personal"
    }
    req = make_request(url, headers=headers)
    try:
        with urllib.request.urlopen(req) as response:
            stats = json.loads(response.read().decode("utf-8"))
            print(json.dumps(stats, indent=4))
    except Exception as e:
        print(f"Error fetching stats: {e}")

if __name__ == "__main__":
    test_chat_completion()
    print_stats()
