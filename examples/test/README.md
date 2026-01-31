# Helix Testing Tool User Guide

## Introduction

The Helix Test Tool is a command-line utility designed to automate the testing of Helix applications. It streamlines the process of validating your app's behavior by running predefined tests, evaluating responses, and generating comprehensive reports. This tool helps ensure that your Helix app performs as expected, making it easier to maintain high-quality applications.

With the Helix Testing Tool, you can:

- Define tests in your helix.yaml file.
- Run tests automatically with a simple command.
- Evaluate responses against expected outputs.
- Generate detailed reports in JSON, HTML, and Markdown formats.
- Upload reports to the Helix platform for easy sharing and analysis.
- Integrate testing into your CI/CD pipelines by utilizing exit codes.

## Writing Tests

Tests are defined within your helix.yaml file under each assistant configuration. Each test consists of one or more steps, each with a prompt and the expected output.

### Structure of a Test

Here's the basic structure of how tests are defined in helix.yaml:

```yaml
assistants:
  - name: assistant_name
    model: model_name
    tests:
      - name: test_name
        steps:
          - prompt: "User input or question."
            expected_output: "Expected assistant response."
```

### Multi-Turn Conversation Tests

Helix supports testing multi-turn conversations, where multiple interactions between the user and assistant are tested in sequence. Each step in a test represents a turn in the conversation, and the session state is maintained between steps.

Here's how to structure a multi-turn test:

```yaml
tests:
  - name: multi_turn_conversation
    steps:
      - prompt: "First user message"
        expected_output: "First expected assistant response"
      - prompt: "Second user message (follows up on first message)"
        expected_output: "Second expected assistant response (should show awareness of previous context)"
      - prompt: "Third user message (continues the conversation)"
        expected_output: "Third expected assistant response (builds on entire conversation history)"
```

The test runner will:
1. Use the same session for all steps in a test
2. Evaluate each turn separately
3. Consider the test successful only if all turns pass
4. Show detailed results for each turn in the test report

For a complete example, see the [multi_turn_test.yaml](../multi_turn_test.yaml) file.

### Example

Let's create an example to illustrate how to write tests. Create a file called `helix.yaml`:

```yaml
assistants:
  - name: math_assistant
    model: llama3:instruct
    tests:
      - name: addition_test
        steps:
          - prompt: "What is 2 + 2?"
            expected_output: "4"
      - name: subtraction_test
        steps:
          - prompt: "What is 5 - 3?"
            expected_output: "2"
```

In this example, we have an assistant named math_assistant using the `llama3:instruct` model. We've defined two tests:

1. addition_test: Checks if the assistant correctly answers "What is 2 + 2?" with "4".
2. subtraction_test: Checks if the assistant correctly answers "What is 5 - 3?" with "2".

Tips for Writing Effective Tests

- Be Specific: Clearly define the prompts and expected outputs.
- Edge Cases: Include tests for edge cases or common failure points.
- Consistent Formatting: Ensure the expected output matches the assistant's expected response format.

### API Integration Example

This is a more complicated example demonstrating an [API
integration](https://docs.helixml.tech/helix/develop/apps/#api-integrations) with Coinbase.

In these tests we test various user scenarios. In one, the user asks for historical data, but the
API doesn't have any historical data. Initially the test will fail because it doesn't know this. But
if you uncomment the `response_success_template` to tell the model that it doesn't have historical
data in the API then the test will pass.

```yaml
name: My Example Helix Weather Bot
description: A bot to keep me up to date with weather information
assistants:
- name: Weather API
  model: llama3:instruct
  apis:
    - name: Weather API
      description: Gets current weather conditions for any location.
      url: https://api.openweathermap.org/data/2.5
      schema: https://raw.githubusercontent.com/helixml/example-app-api-template/refs/heads/main/openapi/weather.yaml
  tests:
  - name: weather_request_1
    steps:
    - prompt: "What's the weather like in London?"
      expected_output: "The current weather in London"
  - name: location_test
    steps:
    - prompt: "Tell me about the weather in New York"
      expected_output: "Weather information for New York"
```

## Running Tests and Generating Results

Prerequisites:

- Helix CLI Installed: Ensure you have the [Helix command-line interface installed](https://docs.helixml.tech/helix/using-helix/client/).
- API Key: Set the `HELIX_API_KEY` environment variable with your Helix API key.
- Helix URL (Optional): If you're using a custom Helix instance, set the `HELIX_URL` environment variable.

### Running Tests

Use the following command to run tests:

```sh
helix test --file helix.yaml
```

- Replace path/to/helix.yaml with the path to your YAML file if it's not in the current directory.
- The tool will read the tests from the specified YAML file, deploy the app, run the tests, and generate reports.

### Understanding the Output

As the tests run, you'll see output in the terminal indicating progress:

- A `.` (dot) represents a passing test.
- An F represents a failing test.

### Loading and Interpreting Results

After the tests have completed, the tool generates and uploads reports. Here's how to access and interpret them.

#### Accessing the Reports

The tool writes reports in three formats:

1. JSON Results: Contains detailed data for each test.
2. HTML Report: An interactive report for viewing in a web browser.
3. Markdown Summary: A summary of test results in Markdown format.

#### Viewing Reports Locally

To view the reports locally:

1. Locate the generated files in your working directory.
2. Open the HTML file in a web browser to view the interactive report.

#### Interpreting the Data

The HTML report provides a comprehensive view of your test results.

- Header: Displays total execution time and the file used for testing.
- Test Results Table:
- Test Name: Name of each test.
- Result: PASS or FAIL.
- Reason: Explanation provided by the evaluation step.
- Model: Model used for the assistant.
- Inference Time: Time taken for the assistant to generate a response.
- Evaluation Time: Time taken to evaluate the response.
- Session Link: Link to the session in the Helix platform.
- Debug Link: Link to debug information in the Helix platform.
- Interactive Features:
- View helix.yaml: Button to view the helix.yaml content used for the tests.
- Tooltips: Hover over truncated text in the "Reason" column to see the full explanation.

#### JSON Results

The JSON file contains structured data for all test results, which can be used for further analysis or integration with other tools.

#### Markdown Summary

The Markdown summary provides a concise overview of the test outcomes, suitable for inclusion in documentation or reports.

#### Investigating Failures

For failing tests:

1. Review the Reason: Understand why the test failed based on the explanation.
2. Use Session and Debug Links: Click the links to see the actual assistant responses and debug logs.
3. Adjust Tests or Assistant Configuration: Based on your findings, you may need to update your tests or modify the assistant's configuration.

## Best Practices

- Regular Testing: Integrate the testing tool into your development workflow to catch issues early.
- Continuous Integration: Use the exit codes to fail builds when tests fail, ensuring code quality.
- Detailed Expected Outputs: Provide precise expected outputs to improve evaluation accuracy.
- Review Reports: Regularly review the generated reports to monitor assistant performance over time.

## Troubleshooting

- API Key Issues: Ensure that the HELIX_API_KEY environment variable is set correctly.
- Network Connectivity: Check your internet connection if requests to the Helix API fail.
- Environment Variables: Confirm that all necessary environment variables are set, including HELIX_URL if using a custom instance.
- Graphical Environment: If the HTML report doesn't open automatically, ensure you're in a graphical environment or open the report manually.

## Example Workflow

1. Set Up Environment:

```sh
export HELIX_API_KEY=your_api_key
```


2. Write Tests in helix.yaml:

```yaml
assistants:
  - name: faq_assistant
    model: llama3:instruct
    tests:
      - name: greeting_test
        steps:
          - prompt: "Hello!"
            expected_output: "Hi there! How can I assist you today?"
```

3. Run Tests:

```sh
helix test --file helix.yaml
```

4. View Output:

```txt
Deployed app with ID: app_abcdef123456
Running tests...
.
Results written to /test-runs/test_id/results_test_id_timestamp.json
HTML report written to /test-runs/test_id/report_test_id_timestamp.html
Summary written to /test-runs/test_id/summary_test_id_timestamp.md
```

5. Open HTML Report

- If not opened automatically, find the HTML file and open it in your web browser.
- Review the test results, reasons, and use session links for deeper investigation.
	
6. Interpret Results

- PASS: The assistant's response matches the expected output.
- FAIL: The response doesn't match; review the reason and consider updating the assistant or test.

7. Update Tests or Assistant

- Make necessary changes based on test outcomes.
- Re-run tests to verify improvements.

## FAQs

- Q: What is the status of this tool?
  A: Helix test is in beta. APIs and usage may change at any time.
- Q: How do I integrate this tool into my CI/CD pipeline?
  A: Since the tool exits with a non-zero status code when tests fail, you can incorporate it into your pipeline scripts to automatically fail builds when tests don't pass.
- Q: Can I test multiple assistants in one helix.yaml file?
  A: Yes, you can define multiple assistants and their respective tests within the same helix.yaml file.
- Q: What models are supported for testing?
  A: You can specify any model supported by Helix in your assistant configuration. Ensure the model name is correct and available.
- Q: How do I handle asynchronous or multi-turn conversations in tests?
  A: Unfortunately this isn't supported yet. Let us know if you're interested!

## Conclusion

The Helix Testing Tool is a powerful utility that simplifies the testing process for Helix applications. By defining tests in your helix.yaml file and using the tool to run them, you can efficiently validate your assistant's behavior, generate informative reports, and maintain high-quality applications.

For further assistance or questions, refer to the [Helix documentation](https://docs.helixml.tech/helix/) or contact support.
