#!/usr/bin/env python3
"""
Simple Aider wrapper that provides basic AI coding assistance
using OpenRouter API without requiring the full Aider CLI
"""

import sys
import os
import json
import time
import requests
from pathlib import Path

class SimpleAider:
    def __init__(self):
        self.api_key = os.getenv("OPENROUTER_API_KEY")
        self.model = os.getenv("OPENROUTER_MODEL", "anthropic/claude-3-haiku")
        self.base_url = "https://openrouter.ai/api/v1"
        self.app_name = "DevTail Gateway"
        
        if self.api_key:
            print(f"Using OpenRouter with model: {self.model}", file=sys.stderr)
        else:
            print("No OPENROUTER_API_KEY found, using mock mode", file=sys.stderr)
        
    def process_request(self, user_input):
        if not self.api_key:
            return self.mock_response(user_input)
            
        try:
            return self.openrouter_request(user_input)
        except Exception as e:
            print(f"OpenRouter API Error: {e}", file=sys.stderr)
            return self.mock_response(user_input)
            
    def openrouter_request(self, user_input):
        system_prompt = """You are an AI coding assistant. Help with code, explain concepts, and suggest improvements. 
Keep responses concise and practical. When making code changes, show the specific lines to modify.
Focus on being helpful and accurate."""
        
        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
            "HTTP-Referer": "https://github.com/devtail/gateway",
            "X-Title": self.app_name
        }
        
        payload = {
            "model": self.model,
            "messages": [
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_input}
            ],
            "max_tokens": 1024,
            "temperature": 0.7
        }
        
        response = requests.post(
            f"{self.base_url}/chat/completions",
            headers=headers,
            json=payload,
            timeout=30
        )
        response.raise_for_status()
        
        result = response.json()
        return result["choices"][0]["message"]["content"]
            
    def mock_response(self, user_input):
        return f"""I understand you're asking about: "{user_input}"

Here's a basic approach:

```python
def solution():
    # Implementation for: {user_input}
    # TODO: Add specific logic here
    return "result"
```

This is a mock response since no API keys are configured. 
To enable real AI assistance, set ANTHROPIC_API_KEY or OPENAI_API_KEY.
"""

def main():
    print("Simple Aider v0.1.0")
    print(f"Model: {os.getenv('AIDER_MODEL', 'auto-detect')}")
    print()
    print("aider> ", end="", flush=True)
    
    aider = SimpleAider()
    
    try:
        while True:
            try:
                line = sys.stdin.readline()
                if not line:
                    break
                    
                line = line.strip()
                if not line:
                    print("aider> ", end="", flush=True)
                    continue
                    
                if line.lower() in ['/exit', '/quit', 'exit', 'quit']:
                    break
                    
                # Process the request
                response = aider.process_request(line)
                
                # Stream the response word by word
                words = response.split()
                for word in words:
                    print(word, end=" ", flush=True)
                    time.sleep(0.02)  # Small delay for streaming effect
                    
                print("\n")
                print("aider> ", end="", flush=True)
                
            except KeyboardInterrupt:
                print("\nExiting...")
                break
            except Exception as e:
                print(f"\nError: {e}", file=sys.stderr)
                print("aider> ", end="", flush=True)
                
    except Exception as e:
        print(f"Fatal error: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()