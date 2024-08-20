const express = require('express');
const bodyParser = require('body-parser');

const app = express();
app.use(bodyParser.json());

const PORT = 5000;

app.post('/api/index', async (req, res) => {
  try {
    const payload = req.body;

    // Implement the logic to index the data here
    
    console.log("data_entity_id: " + payload.data_entity_id);
    console.log("content: " + payload.content);

    res.json({ "status": "ok" });
  } catch (error) {
    res.status(400).json({ error: error.message });
  }
});

app.post('/api/query', async (req, res) => {
  try {
    const { prompt, data_entity_id, distance_threshold, distance_function, max_results } = req.body;

    if (!prompt || prompt.length === 0) {
      throw new Error('missing prompt');
    }
    if (!data_entity_id || data_entity_id.length === 0) {
      throw new Error('missing data_entity_id');
    }

    console.log("prompt: " + prompt);
    console.log("data_entity_id: " + data_entity_id);
    console.log("distance_threshold: " + distance_threshold);
    console.log("distance_function: " + distance_function);
    console.log("max_results: " + max_results);

    // Implement the logic to query the data here
    

    res.json([
      {
        "id": "1",
        "document_id": "1",
        "content": "Kai has a red car",
        "distance": 0.1                
      },
      {
        "id": "2",
        "document_id": "2",
        "content": "Karolis has a blue car",
        "distance": 0.1                
      },
      {
        "id": "2",
        "document_id": "2",
        "content": "Luke has a white car",
        "distance": 0.1                
      }
    ]);
  } catch (error) {
    res.status(400).json({ error: error.message });
  }
});

app.listen(PORT, '0.0.0.0', () => {
  console.log(`Server running on http://0.0.0.0:${PORT}`);
});