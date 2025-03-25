# Running API integration tests

1. Prepare your .env file like you have for a working local dev setup
2. Set the environment variables from it:

```bash
export $(cat .env | xargs)
```

3. Run the tests:

```bash
./stack test-integration
```
