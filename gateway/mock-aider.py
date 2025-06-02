#!/usr/bin/env python3
"""Mock Aider for testing the gateway"""

import sys
import time
import random

def main():
    print("Aider v0.1.0 (mock)")
    print("Model: claude-3-sonnet")
    print()
    
    while True:
        line = sys.stdin.readline()
        if not line:
            break
            
        line = line.strip()
        if not line:
            continue
            
        # Simulate thinking
        print(f"Thinking about: {line}")
        time.sleep(0.5)
        
        # Stream a response
        response = f"I understand you want help with '{line}'. "
        response += "Here's a simple implementation:\n\n```python\n"
        response += "def solution():\n"
        response += f"    # Solution for: {line}\n"
        response += "    return 'Hello from mock Aider!'\n"
        response += "```\n"
        
        # Stream tokens
        words = response.split()
        for i, word in enumerate(words):
            print(word, end=' ', flush=True)
            time.sleep(0.05 + random.random() * 0.05)
        
        print("\naider> ", end='', flush=True)

if __name__ == "__main__":
    main()