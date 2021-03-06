# Copyright (C) The Arvados Authors. All rights reserved.
#
# SPDX-License-Identifier: AGPL-3.0

# Perform api_token checking very early in the request process.  We want to do
# this in the Rack stack instead of in ApplicationController because
# websockets needs access to authentication but doesn't use any of the rails
# active dispatch infrastructure.
class ArvadosApiToken

  # Create a new ArvadosApiToken handler
  # +app+  The next layer of the Rack stack.
  def initialize(app = nil, options = nil)
    @app = app.respond_to?(:call) ? app : nil
  end

  def call env
    # First, clean up just in case we have a multithreaded server and thread
    # local variables are still set from a prior request.  Also useful for
    # tests that call this code to set up the environment.
    Thread.current[:api_client_ip_address] = nil
    Thread.current[:api_client_authorization] = nil
    Thread.current[:api_client_uuid] = nil
    Thread.current[:api_client] = nil
    Thread.current[:user] = nil

    request = Rack::Request.new(env)
    params = request.params
    remote_ip = env["action_dispatch.remote_ip"]

    Thread.current[:request_starttime] = Time.now
    user = nil
    api_client = nil
    api_client_auth = nil
    if request.get? || params["_method"] == 'GET'
      reader_tokens = params["reader_tokens"]
      if reader_tokens.is_a? String
        reader_tokens = SafeJSON.load(reader_tokens)
      end
    else
      reader_tokens = nil
    end

    # Set current_user etc. based on the primary session token if a
    # valid one is present. Otherwise, use the first valid token in
    # reader_tokens.
    [params["api_token"],
     params["oauth_token"],
     env["HTTP_AUTHORIZATION"].andand.match(/OAuth2 ([a-zA-Z0-9]+)/).andand[1],
     *reader_tokens,
    ].each do |supplied|
      next if !supplied
      try_auth = ApiClientAuthorization.
        includes(:api_client, :user).
        where('api_token=? and (expires_at is null or expires_at > CURRENT_TIMESTAMP)', supplied).
        first
      if try_auth.andand.user
        api_client_auth = try_auth
        user = api_client_auth.user
        api_client = api_client_auth.api_client
        break
      end
    end
    Thread.current[:api_client_ip_address] = remote_ip
    Thread.current[:api_client_authorization] = api_client_auth
    Thread.current[:api_client_uuid] = api_client.andand.uuid
    Thread.current[:api_client] = api_client
    Thread.current[:user] = user

    @app.call env if @app
  end
end
