# Generated by the gRPC Python protocol compiler plugin. DO NOT EDIT!
"""Client and server classes corresponding to protobuf-defined services."""
import grpc

import hook_pb2 as hook__pb2


class HookHandlerStub(object):
    """The hook service definition.
    """

    def __init__(self, channel):
        """Constructor.

        Args:
            channel: A grpc.Channel.
        """
        self.InvokeHook = channel.unary_unary(
                '/v2.HookHandler/InvokeHook',
                request_serializer=hook__pb2.HookRequest.SerializeToString,
                response_deserializer=hook__pb2.HookResponse.FromString,
                )


class HookHandlerServicer(object):
    """The hook service definition.
    """

    def InvokeHook(self, request, context):
        """InvokeHook is invoked for every hook that is executed. HookRequest contains the
        corresponding information about the hook type, the involved upload, and
        causing HTTP request.
        The return value HookResponse allows to stop or reject an upload, as well as modifying
        the HTTP response. See the documentation for HookResponse for more details.
        """
        context.set_code(grpc.StatusCode.UNIMPLEMENTED)
        context.set_details('Method not implemented!')
        raise NotImplementedError('Method not implemented!')


def add_HookHandlerServicer_to_server(servicer, server):
    rpc_method_handlers = {
            'InvokeHook': grpc.unary_unary_rpc_method_handler(
                    servicer.InvokeHook,
                    request_deserializer=hook__pb2.HookRequest.FromString,
                    response_serializer=hook__pb2.HookResponse.SerializeToString,
            ),
    }
    generic_handler = grpc.method_handlers_generic_handler(
            'v2.HookHandler', rpc_method_handlers)
    server.add_generic_rpc_handlers((generic_handler,))


 # This class is part of an EXPERIMENTAL API.
class HookHandler(object):
    """The hook service definition.
    """

    @staticmethod
    def InvokeHook(request,
            target,
            options=(),
            channel_credentials=None,
            call_credentials=None,
            insecure=False,
            compression=None,
            wait_for_ready=None,
            timeout=None,
            metadata=None):
        return grpc.experimental.unary_unary(request, target, '/v2.HookHandler/InvokeHook',
            hook__pb2.HookRequest.SerializeToString,
            hook__pb2.HookResponse.FromString,
            options, channel_credentials,
            insecure, call_credentials, compression, wait_for_ready, timeout, metadata)
