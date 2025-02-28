# Haystack Service Tests

This directory contains unit tests for the Haystack service.

## Unix Socket Embedders Tests

The `test_unix_socket_embedders.py` file contains tests for the Unix socket embedders. These tests verify that the embedders correctly use a Unix socket for communication with the OpenAI API, rather than making HTTPS connections to api.openai.com.

### How the Tests Work

The tests create a mock Unix socket server that responds to requests with predefined responses. This allows us to test the embedders without making actual API calls to OpenAI.

The tests verify:
1. The `UnixSocketAdapter` correctly connects to the socket
2. The embedders correctly initialize with the socket path
3. The embedders use the Unix socket for embedding
4. The embedders can embed text and documents using the mock server

### Running the Tests

To run the tests, you need to have the Haystack service dependencies installed. First, activate the virtual environment:

```bash
# From the haystack_service directory
. venv/bin/activate
```

Then, you can install the dependencies if you haven't already:

```bash
pip install -r requirements.txt
```

Now you can run the tests with:

```bash
# From the haystack_service directory
python -m unittest discover -s tests -p "test_*.py"
```

Or run a specific test file:

```bash
# From the haystack_service directory
python -m unittest tests/test_unix_socket_embedders.py
```

## Debugging the Unix Socket Connection

If you're having issues with the Unix socket connection, you can use the debug script to help diagnose the problem:

```bash
# From the haystack_service directory
. venv/bin/activate
python tests/debug_unix_socket.py
```

The debug script creates a mock server that logs all requests it receives, which can help you understand what's happening with the Unix socket connection. It will show you:

1. Whether the HTTP client is correctly set to use the Unix socket adapter
2. What the base URL is set to (should be "http://localhost")
3. Whether requests are being sent to the Unix socket or to api.openai.com
4. The content of the requests and responses

## Adding More Tests

To add more tests, create a new test file in this directory with a name starting with `test_`. The tests will be automatically discovered and run by the unittest discovery mechanism. 