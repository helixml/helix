## data entities

These are files that have some role to play in the process of:

 * fine tuning input data
 * fine tuning output artifacts
 * RAG sources
 * inference uploaded files
   * e.g. upload a CSV and ask questions about it
   * e.g. upload an image and ask what is inside it

Here is a list of the different data entities:

 * `documents` - original files uploaded by the user for text fine tuning
 * `images` - original files uploaded by the user for image fine tuning
 * `plain_text` - `documents` converted to plain text
 * `qa_pairs` - a JSON file(s) of question-answer pairs - we turn `plain_text` into these using an LLM
 * `finetune` - an artifact prouced by fine tuning a model
 * `rag` - a database that holds a vector representation of a `plain_text` data entity

Some of these entities are produced by converting others.

Some of them are used an inputs to finetune sessions.

Others are the outputs of finetune sessions.

The interface for a data entity pipeline is:

```go
type DataEntity struct {
  // whatever fields the data entity needs go here
}
type DataEntityTransformer interface {
	Transform(ctx context.Context, entity DataEntity) (DataEntity, error)
}
```

Then we can have various pipelines by doing this:

```go
func RunPipeline(ctx context.Context, entity DataEntity, transformers ...DataEntityTransformer) (DataEntity, error) {
  for _, transformer := range transformers {
    entity, err := transformer.Transform(ctx, entity)
    if err != nil {
      return nil, err
    }
  }
  return entity, nil
}
```

The model should produce the pipeline - each model will have it's own opinions about what data prep looks like.

Also - the user should be able to shortcut to any part of the pipeline.

For example, they might already have prepared a `qa_pairs` file - so now it's just create a data entity of type `qa_pairs` and then run the pipeline from the finetune stage.

The finetune stage is just a `Transform` function - it turns one `DataEntity` into another (`qa_pairs` -> `finetune`)