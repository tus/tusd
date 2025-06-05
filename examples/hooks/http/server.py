from http.server import HTTPServer, BaseHTTPRequestHandler
from io import BytesIO

import json
import time
import uuid

class HTTPHookHandler(BaseHTTPRequestHandler):

    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'Hello! This server only responds to POST requests')

    def do_POST(self):
        # Read entire body as JSON object
        content_length = int(self.headers['Content-Length'])
        request_body = self.rfile.read(content_length)
        hook_request = json.loads(request_body)

        # Print data from hook request for debugging
        print('Received hook request:')
        print(hook_request)

        # Prepare hook response structure
        hook_response = {
            'HTTPResponse': {
                'Headers': {}
            }
        }

        # Example: Use the pre-create hook to check if a filename has been supplied
        # using metadata. If not, the upload is rejected with a custom HTTP response.
        # In addition, a custom upload ID with a choosable prefix is supplied.
        # Metadata is configured, so that it only retains the filename meta data
        # and the creation time.
        if hook_request['Type'] == 'pre-create':
            metaData = hook_request['Event']['Upload']['MetaData']
            isValid = 'filename' in metaData
            if not isValid:
                hook_response['RejectUpload'] = True
                hook_response['HTTPResponse']['StatusCode'] = 400
                hook_response['HTTPResponse']['Body'] = 'no filename provided'
                hook_response['HTTPResponse']['Headers']['X-Some-Header'] = 'yes'
            else:
                hook_response['ChangeFileInfo'] = {}
                hook_response['ChangeFileInfo']['ID'] = f'prefix-{uuid.uuid4()}' 
                hook_response['ChangeFileInfo']['MetaData'] = {
                    'filename': metaData['filename'],
                    'creation_time': time.ctime(),
                }


        # Example: Use the post-finish hook to print information about a completed upload,
        # including its storage location.
        if hook_request['Type'] == 'post-finish':
            id      = hook_request['Event']['Upload']['ID']
            size    = hook_request['Event']['Upload']['Size']
            storage = hook_request['Event']['Upload']['Storage']

            print(f'Upload {id} ({size} bytes) is finished. Find the file at:')
            print(storage)


        # Print data of hook response for debugging
        print('Responding with hook response:')
        print(hook_response)
        print('------')
        print('')

        # Send the data from the hook response as JSON output
        response_body = json.dumps(hook_response)
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        self.wfile.write(response_body.encode())


httpd = HTTPServer(('localhost', 8000), HTTPHookHandler)
httpd.serve_forever()
