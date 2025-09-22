#!/usr/bin/env python3
"""
Mock WebSocket server to test Zed WebSocket sync without Helix infrastructure
"""

import asyncio
import websockets
import json
import sys

async def handle_websocket(websocket, path):
    print(f"🔌 New WebSocket connection from {websocket.remote_address}")
    
    try:
        async for message in websocket:
            try:
                data = json.loads(message)
                print(f"📥 Received: {data}")
                
                if data.get("type") == "register":
                    # Respond to registration
                    response = {
                        "type": "registration_success",
                        "agent_id": data.get("agent_id"),
                        "message": "Agent registered successfully"
                    }
                    await websocket.send(json.dumps(response))
                    print(f"📤 Sent registration response: {response}")
                    
                    # After a short delay, send a chat message to trigger thread creation
                    await asyncio.sleep(2)
                    chat_message = {
                        "type": "chat_message",
                        "helix_session_id": "test-session-123",
                        "message": "Hello from mock Helix! This should create a thread in Zed.",
                        "request_id": "req-123"
                    }
                    await websocket.send(json.dumps(chat_message))
                    print(f"📤 Sent chat message: {chat_message}")
                
                elif data.get("type") == "response":
                    print(f"✅ Received response from Zed: {data.get('content')}")
                    
            except json.JSONDecodeError:
                print(f"❌ Invalid JSON received: {message}")
                
    except websockets.exceptions.ConnectionClosed:
        print("🔌 WebSocket connection closed")
    except Exception as e:
        print(f"❌ Error handling WebSocket: {e}")

async def main():
    print("🚀 Starting mock WebSocket server on ws://localhost:8080/api/v1/external-agents/sync")
    print("This will test Zed WebSocket sync without requiring Helix infrastructure")
    
    server = await websockets.serve(
        handle_websocket, 
        "localhost", 
        8080,
        subprotocols=["external-agent-sync"]
    )
    
    print("✅ Mock WebSocket server started!")
    print("Now start Zed with WebSocket sync enabled and watch for connections...")
    
    await server.wait_closed()

if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        print("\n🛑 Mock WebSocket server stopped")
        sys.exit(0)
