const express = require('express');
const bodyParser = require('body-parser');

const app = express();
app.use(bodyParser.json());

const PORT = 5000;

// Simulate a database
let data = {}

// Index the data based on the data entity id
app.post('/api/index', async (req, res) => {
  try {
    const payload = req.body;

    // Implement the logic to index the data here
    
    console.log("data_entity_id: " + payload.data_entity_id);
    console.log("content: " + payload.content);

    data[payload.data_entity_id] = payload.content;

    res.json({ "status": "ok" });
  } catch (error) {
    res.status(400).json({ error: error.message });
  }
});

// Delete the indexed data for this data entity id
app.delete('/api/index/:data_entity_id', async (req, res) => {
  console.log("deleting data_entity_id: " + req.params.data_entity_id);
  try {
    const { data_entity_id } = req.params;
    delete data[data_entity_id];
    res.json({ "status": "ok" });
  } catch (error) {
    res.status(400).json({ error: error.message });
  }
});

// Query the indexed data based on the data entity id and prompt
app.post('/api/index/query', async (req, res) => {  
  try {
    const { prompt, data_entity_id, distance_threshold, distance_function, max_results } = req.body;

    if (!prompt || prompt.length === 0) {
      throw new Error('missing prompt');
    }
    if (!data_entity_id || data_entity_id.length === 0) {
      throw new Error('missing data_entity_id');
    }

    console.log("querying data for data_entity_id: " + data_entity_id);

    // Implement the logic to query the data here
    const content = data[data_entity_id];

    console.log("prompt: " + prompt);
    console.log("data_entity_id: " + data_entity_id);
    console.log("distance_threshold: " + distance_threshold);
    console.log("distance_function: " + distance_function);
    console.log("max_results: " + max_results);  

    res.json([
      {
        "id": "1",
        "document_id": "1",
        "content": content,
        "distance": 0.1                
      },      
    ]);
  } catch (error) {
    res.status(400).json({ error: error.message });
  }
});

app.listen(PORT, '0.0.0.0', () => {
  console.log(`Server running on http://0.0.0.0:${PORT}`);
});