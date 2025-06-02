#!/usr/bin/env python3
"""Simple echo server that mimics Aider's behavior for testing"""

import sys
import time

def main():
    print("Mock Aider v0.1.0")
    print("Ready to assist with your coding tasks.")
    print()
    sys.stdout.flush()
    
    while True:
        try:
            line = sys.stdin.readline()
            if not line:
                break
                
            line = line.strip()
            if not line:
                continue
            
            # Echo back with some formatting
            print(f"I received your message: '{line}'")
            print()
            print("Here's a simple response to demonstrate streaming:")
            
            response = f"You said: {line}. This is a test response that will be streamed word by word."
            words = response.split()
            
            for word in words:
                print(word, end=' ', flush=True)
                time.sleep(0.1)  # Simulate streaming delay
            
            print("\n\naider> ", end='', flush=True)
            
        except Exception as e:
            print(f"\nError: {e}", file=sys.stderr)
            break

if __name__ == "__main__":
    main()