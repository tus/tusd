import grpc
from concurrent import futures
import time
import hook_pb2_grpc as pb2_grpc
import hook_pb2 as pb2

class HookHandler(pb2_grpc.HookHandlerServicer):

    def __init__(self, *args, **kwargs):
        pass

    def InvokeHook(self, hook_request, context):
        # Print data from hook request for debugging
        print('Received hook request:')
        print(hook_request)

        # Prepare hook response structure
        hook_response = pb2.HookResponse()

        # Example: Use the pre-create hook to check if a filename has been supplied
        # using metadata. If not, the upload is rejected with a custom HTTP response.
        if hook_request.type == 'pre-create':
            filename = hook_request.event.upload.metaData['filename']
            if filename == "":
                hook_response.rejectUpload = True
                hook_response.httpResponse.statusCode = 400
                hook_response.httpResponse.body = 'no filename provided'
                hook_response.httpResponse.headers['X-Some-Header'] = 'yes'

        # Example: Use the post-finish hook to print information about a completed upload,
        # including its storage location.
        if hook_request.type == 'post-finish':
            id      = hook_request.event.upload.id
            size    = hook_request.event.upload.size
            storage = hook_request.event.upload.storage

            print(f'Upload {id} ({size} bytes) is finished. Find the file at:')
            print(storage)

        # Print data of hook response for debugging
        print('Responding with hook response:')
        print(hook_response)
        print('------')
        print('')

        # Return the hook response to send back to tusd
        return hook_response

def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    pb2_grpc.add_HookHandlerServicer_to_server(HookHandler(), server)
    server.add_insecure_port('[::]:8000')
    server.start()
    server.wait_for_termination()

if __name__ == '__main__':
    serve()
