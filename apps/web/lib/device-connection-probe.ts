import http from "node:http";
import https from "node:https";
import net from "node:net";
import tls from "node:tls";

import type { DeviceEndpointProbe } from "./device-connection-check-core.ts";

const defaultPorts: Record<string, number> = {
  "http:": 80,
  "https:": 443,
  "ws:": 80,
  "wss:": 443,
  "mqtt:": 1883,
  "mqtts:": 8883,
  "rtmp:": 1935,
  "rtmps:": 1936
};

function withTimeout<T>(start: (settle: (error?: Error) => void) => T, timeoutMs: number) {
  return new Promise<void>((resolve, reject) => {
    let resource: T | undefined;
    const timer = setTimeout(() => {
      if (resource && typeof resource === "object" && "destroy" in resource) {
        (resource as { destroy: () => void }).destroy();
      }
      reject(new Error("ENDPOINT_PROBE_TIMEOUT"));
    }, timeoutMs);
    const settle = (error?: Error) => {
      clearTimeout(timer);
      if (error) reject(error); else resolve();
    };
    resource = start(settle);
  });
}

function probeSocket(endpoint: Parameters<DeviceEndpointProbe>[0], timeoutMs: number) {
  const address = endpoint.addresses[0];
  const port = Number(endpoint.url.port || defaultPorts[endpoint.url.protocol]);
  const secure = ["mqtts:", "wss:", "rtmps:"].includes(endpoint.url.protocol);
  return withTimeout((settle) => {
    const socket = secure
      ? tls.connect({ host: address.address, port, servername: endpoint.url.hostname, rejectUnauthorized: true })
      : net.connect({ host: address.address, port });
    socket.once(secure ? "secureConnect" : "connect", () => {
      socket.destroy();
      settle();
    });
    socket.once("error", () => settle(new Error("ENDPOINT_PROBE_FAILED")));
    return socket;
  }, timeoutMs);
}

function probeHttp(endpoint: Parameters<DeviceEndpointProbe>[0], timeoutMs: number) {
  const address = endpoint.addresses[0];
  const secure = endpoint.url.protocol === "https:";
  const client = secure ? https : http;
  return withTimeout((settle) => {
    const request = client.request(endpoint.url, {
      method: "HEAD",
      lookup: (_hostname, _options, callback) => callback(null, address.address, address.family),
      ...(secure ? { servername: endpoint.url.hostname, rejectUnauthorized: true } : {})
    }, (response) => {
      response.resume();
      if ((response.statusCode ?? 500) < 500) settle();
      else settle(new Error("ENDPOINT_UNHEALTHY"));
    });
    request.once("error", () => settle(new Error("ENDPOINT_PROBE_FAILED")));
    request.end();
    return request;
  }, timeoutMs);
}

export function createDeviceEndpointProbe(timeoutMs = 5_000): DeviceEndpointProbe {
  return async (endpoint) => {
    if (endpoint.url.protocol === "http:" || endpoint.url.protocol === "https:") {
      await probeHttp(endpoint, timeoutMs);
      return;
    }
    await probeSocket(endpoint, timeoutMs);
  };
}
