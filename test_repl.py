#!/usr/bin/env python3
"""
Test script to verify REPL commands work correctly.
This script simulates user input to the REPL mode.
"""

import subprocess
import time
import sys

def test_repl_commands():
    # Test /help command
    print("Testing /help command...")
    process = subprocess.Popen(
        ['./esa', '--repl'],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        cwd='/Users/meain/dev/src/esa'
    )
    
    # Send /help command followed by empty line and then /exit
    input_text = "/help\n\n/exit\n\n"
    stdout, stderr = process.communicate(input_text, timeout=10)
    
    print("STDERR output:")
    print(stderr)
    print("\nSTDOUT output:")
    print(stdout)
    
    # Check if help text appears in output
    if "Available commands:" in stderr:
        print("✓ /help command works!")
    else:
        print("✗ /help command failed!")
    
    return process.returncode == 0

if __name__ == "__main__":
    try:
        success = test_repl_commands()
        sys.exit(0 if success else 1)
    except Exception as e:
        print(f"Test failed with error: {e}")
        sys.exit(1)
