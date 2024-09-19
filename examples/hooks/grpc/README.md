# Example gRPC hook server with Python

Use the following commands to run this example in a virtual environment:

```sh
# Setup virtual environment and install dependencies
python3 -m venv venv
source venv/bin/activate
pip3 install -r requirements.txt

# Build gRPC code, if necessary
make -B hook_pb2.py

# Start gRPC server (listening at localhost:8000)
python3 server.py

# In a separate terminal you can now run tusd and point it to the gRPC server
tusd -hooks-grpc localhost:8000
```
