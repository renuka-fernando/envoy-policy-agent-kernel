# envoy-policy-agent-kernel
Agent Kernel of Policy Engine

### Run Setup

Start Ext Processing Server:
```sh
cd policy-engine; go run main.go; cd -
```

Start Envoy and the backend service:
```sh
docker compose down; docker compose up -d; docker compose logs -ft
```

### Test: Post Request

```sh
curl 'http://localhost:8000/pets/myPetId123/history?bar=baz&param_to_remove=bbbbb' -d '{
   "name": "John Doe", "age": 30, "address": "123 Main St, San Francisco, CA 94123", "phone": "123-456-7890", "email": "john@abc.com"
}' -H 'foo: hello-foo1' -H 'foo: hello-foo2' -iv -H 'remove-this-header: foooo'
```

### Test: Chunked Request

```sh
curl 'http://localhost:8000/pets/myPetId123/history?bar=baz&param_to_remove=bbbbb' -d @10KB.json -H 'foo: hello-foo' -H 'Transfer-Encoding: chunked' -iv
```

### Stop

```sh
docker compose down
```

