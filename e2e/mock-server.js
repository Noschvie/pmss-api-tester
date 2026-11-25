// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 Noschvie
// PMSS API Tester - https://github.com/Noschvie/pmss-api-tester

// Local test double only: it is not a Keycloak or PMSS implementation.
const http = require('node:http');
const crypto = require('node:crypto');
const { URL } = require('node:url');

const origin = 'http://127.0.0.1:4174';
const issuer = `${origin}/realms/test`;
const callback = 'http://127.0.0.1:4173/auth/callback';
const { privateKey, publicKey } = crypto.generateKeyPairSync('rsa', { modulusLength: 2048 });
const jwk = { ...publicKey.export({ format: 'jwk' }), kid: 'e2e-key', use: 'sig', alg: 'RS256' };
const codes = new Map();
const subjectTokens = new Map();
let requests = [];

function json(res, status, value, headers = {}) {
  res.writeHead(status, { 'content-type': 'application/json', ...headers });
  res.end(JSON.stringify(value));
}

function redirect(res, url) {
  res.writeHead(303, { location: url });
  res.end();
}

function signedIDToken(nonce, username) {
  const now = Math.floor(Date.now() / 1000);
  const encode = (value) => Buffer.from(JSON.stringify(value)).toString('base64url');
  const content = `${encode({ alg: 'RS256', kid: 'e2e-key', typ: 'JWT' })}.${encode({
    iss: issuer, sub: `${username}-id`, aud: 'e2e-client',
    iat: now, exp: now + 3600, nonce, preferred_username: username,
  })}`;
  return `${content}.${crypto.sign('RSA-SHA256', Buffer.from(content), privateKey).toString('base64url')}`;
}

async function form(req) {
  const chunks = [];
  for await (const chunk of req) chunks.push(chunk);
  return new URLSearchParams(Buffer.concat(chunks).toString());
}

function authorizedClient(req, params) {
  const basic = 'Basic ' + Buffer.from('e2e-client:e2e-secret').toString('base64');
  return req.headers.authorization === basic ||
    (params.get('client_id') === 'e2e-client' && params.get('client_secret') === 'e2e-secret');
}

const server = http.createServer(async (req, res) => {
  try {
    const url = new URL(req.url, origin);
    if (url.pathname === '/test/health' && req.method === 'GET') return json(res, 200, { ok: true });
    if (url.pathname === '/test/requests' && req.method === 'GET') return json(res, 200, requests);
    if (url.pathname === '/test/requests' && req.method === 'DELETE') {
      requests = [];
      return json(res, 200, { ok: true });
    }
    if (url.pathname === '/realms/test/.well-known/openid-configuration' && req.method === 'GET') {
      return json(res, 200, {
        issuer, authorization_endpoint: `${issuer}/authorize`, token_endpoint: `${issuer}/token`,
        jwks_uri: `${issuer}/jwks`, end_session_endpoint: `${issuer}/logout`,
        response_types_supported: ['code'], subject_types_supported: ['public'],
        id_token_signing_alg_values_supported: ['RS256'],
      });
    }
    if (url.pathname === '/realms/test/jwks' && req.method === 'GET') return json(res, 200, { keys: [jwk] });
    if (url.pathname === '/realms/test/authorize' && req.method === 'GET') {
      const q = url.searchParams;
      if (q.get('client_id') !== 'e2e-client' || q.get('redirect_uri') !== callback ||
          q.get('response_type') !== 'code' || q.get('code_challenge_method') !== 'S256' ||
          !q.get('state') || !q.get('nonce') || !q.get('code_challenge')) {
        return json(res, 400, { error: 'invalid_authorization_request' });
      }
      res.writeHead(200, { 'content-type': 'text/html; charset=utf-8' });
      return res.end(`<!doctype html><title>Test Keycloak</title><h1>Test Keycloak</h1>
        <form method="post"><label>Username <input name="username" required></label>
        <label>Password <input name="password" type="password" required></label>
        <button type="submit">Sign in</button></form>`);
    }
    if (url.pathname === '/realms/test/authorize' && req.method === 'POST') {
      const q = url.searchParams;
      const params = await form(req);
      const username = params.get('username');
      if (!['alice', 'no-read'].includes(username) || params.get('password') !== 'test-password' ||
          q.get('client_id') !== 'e2e-client' || q.get('redirect_uri') !== callback ||
          q.get('code_challenge_method') !== 'S256' || !q.get('nonce') || !q.get('state') || !q.get('code_challenge')) {
        return json(res, 400, { error: 'invalid_login' });
      }
      const code = crypto.randomBytes(24).toString('base64url');
      codes.set(code, { username, nonce: q.get('nonce'), challenge: q.get('code_challenge') });
      const dest = new URL(callback);
      dest.searchParams.set('state', q.get('state'));
      dest.searchParams.set('code', code);
      return redirect(res, dest.toString());
    }
    if (url.pathname === '/realms/test/token' && req.method === 'POST') {
      const params = await form(req);
      if (!authorizedClient(req, params)) return json(res, 401, { error: 'invalid_client' });
      if (params.get('grant_type') === 'authorization_code') {
        const code = codes.get(params.get('code'));
        codes.delete(params.get('code'));
        const challenge = crypto.createHash('sha256').update(params.get('code_verifier') || '').digest('base64url');
        if (!code || code.challenge !== challenge || params.get('redirect_uri') !== callback) {
          return json(res, 400, { error: 'invalid_grant' });
        }
        const accessToken = crypto.randomBytes(24).toString('base64url');
        subjectTokens.set(accessToken, code.username);
        return json(res, 200, {
          access_token: accessToken, token_type: 'Bearer', expires_in: 3600,
          id_token: signedIDToken(code.nonce, code.username),
        });
      }
      if (params.get('grant_type') === 'urn:ietf:params:oauth:grant-type:token-exchange') {
        const username = subjectTokens.get(params.get('subject_token'));
        if (!username || params.get('audience') !== 'e2e-pmss' || params.get('scope') !== 'read' ||
            params.get('subject_token_type') !== 'urn:ietf:params:oauth:token-type:access_token') {
          return json(res, 400, { error: 'invalid_request' });
        }
        return json(res, 200, {
          access_token: username === 'alice' ? 'e2e-read-token' : 'e2e-no-read-token',
          token_type: 'Bearer', expires_in: 1800, scope: username === 'alice' ? 'read' : 'openid',
        });
      }
      return json(res, 400, { error: 'unsupported_grant_type' });
    }
    if (url.pathname === '/realms/test/logout' && req.method === 'GET') {
      if (url.searchParams.get('post_logout_redirect_uri') !== 'http://127.0.0.1:4173/' ||
          !url.searchParams.get('id_token_hint')) return json(res, 400, { error: 'invalid_logout' });
      return redirect(res, 'http://127.0.0.1:4173/');
    }
    if (url.pathname === '/healthz' || url.pathname.startsWith('/api/')) {
      requests.push({ method: req.method, path: url.pathname, authorization: req.headers.authorization || '' });
      if (req.method !== 'GET') return json(res, 405, { error: 'write request reached mock target' });
      if (url.pathname === '/healthz') return json(res, 200, { status: 'ok' });
      if (url.pathname === '/api/v1/info') {
        if (req.headers.authorization !== 'Bearer e2e-read-token') {
          return json(res, 401, { title: 'Unauthorized', status: 401, code: 'invalid_token' },
            { 'content-type': 'application/problem+json' });
        }
        return json(res, 200, {
          apiVersion: '1.0', implementation: { name: 'PMSS', version: '2.1' },
          broker: { product: 'Mosquitto', version: '2' },
          managed: { rolePrefix: 'pmss_', groupPrefix: 'pmss_', clientMarker: 'managed-by: pmss' },
          passwordPolicy: { length: 24, alphabet: 'A-Za-z0-9' }, reservedNames: [],
          limits: { maxRequestBytes: 1048576 },
        }, { 'cache-control': 'private, no-store' });
      }
    }
    json(res, 404, { error: 'not_found' });
  } catch (error) {
    console.error(error);
    if (!res.headersSent) json(res, 500, { error: 'mock_failure' });
    else res.destroy(error);
  }
});

server.listen(4174, '127.0.0.1', () => console.log('E2E mocks listening on 127.0.0.1:4174'));
