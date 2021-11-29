from http.server import HTTPServer, BaseHTTPRequestHandler

from io import BytesIO

import json


class SimpleHTTPRequestHandler(BaseHTTPRequestHandler):

    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'Hello, world!')

    def do_POST(self):
        content_length = int(self.headers['Content-Length'])
        request_body = self.rfile.read(content_length)

        hook_request = json.loads(request_body)
        print(hook_request)

        hook_response = {
            'HTTPResponse': {
                'Headers': {}
            }
        }

        if hook_request['Type'] == 'pre-create':
            hook_response['HTTPResponse']['Headers']['X-From-Pre-Create'] = 'hello'

            hook_response['RejectUpload'] = True


        if hook_request['Type'] == 'pre-finish':
            hook_response['HTTPResponse']['StatusCode'] = 200
            hook_response['HTTPResponse']['Headers']['X-From-Pre-Finish'] = 'hello again'
            hook_response['HTTPResponse']['Body'] = 'some information'

        response_body = json.dumps(hook_response)
        print(response_body)

        self.send_response(200)
        self.end_headers()
        self.wfile.write(response_body.encode())


httpd = HTTPServer(('localhost', 8000), SimpleHTTPRequestHandler)
httpd.serve_forever()
