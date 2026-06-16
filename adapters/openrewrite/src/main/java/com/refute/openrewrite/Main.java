package com.refute.openrewrite;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.io.PrintWriter;

/**
 * Entry point for the OpenRewrite JSON-RPC adapter.
 *
 * Reads one JSON-RPC request per line from stdin, writes one JSON response per
 * line to stdout. Stderr is available for diagnostic logging. Every request and
 * response carries a "protocolVersion" field in the JSON-RPC envelope; a request
 * whose version does not match PROTOCOL_VERSION is rejected rather than
 * executed. See docs/specs/adapter-wire-contracts.md.
 *
 * Supported methods: rename
 */
public class Main {
    // PROTOCOL_VERSION must match the Go driver's openrewrite.ProtocolVersion.
    static final int PROTOCOL_VERSION = 1;

    public static void main(String[] args) throws Exception {
        ObjectMapper mapper = new ObjectMapper();
        RenameHandler renameHandler = new RenameHandler(mapper);
        BufferedReader reader = new BufferedReader(new InputStreamReader(System.in));
        PrintWriter writer = new PrintWriter(System.out, true);

        String line;
        while ((line = reader.readLine()) != null) {
            String response = handleLine(line, mapper, renameHandler);
            if (response != null) {
                writer.println(response);
            }
        }
    }

    /**
     * Processes a single request line and returns the JSON response string, or
     * null for a blank line. Pure (no I/O) so the wire contract can be exercised
     * directly from tests against the shared golden fixtures.
     */
    static String handleLine(String line, ObjectMapper mapper, RenameHandler renameHandler) {
        line = line.trim();
        if (line.isEmpty()) {
            return null;
        }

        JsonNode req;
        try {
            req = mapper.readTree(line);
        } catch (Exception e) {
            return errorJson(mapper, null, -32700, "Parse error: " + e.getMessage());
        }

        JsonNode idNode = req.get("id");
        Integer id = idNode != null && !idNode.isNull() ? idNode.asInt() : null;
        String method = req.has("method") ? req.get("method").asText() : null;
        JsonNode params = req.get("params");

        JsonNode versionNode = req.get("protocolVersion");
        int protocolVersion = versionNode != null && versionNode.isInt() ? versionNode.asInt() : 0;
        if (protocolVersion != PROTOCOL_VERSION) {
            // -32600 is JSON-RPC "Invalid Request". Reject skewed protocols
            // instead of executing them.
            return errorJson(mapper, id, -32600,
                    "unsupported protocol version: got " + protocolVersion + ", want " + PROTOCOL_VERSION);
        }

        try {
            Object result = dispatch(method, params, renameHandler);
            ObjectNode response = mapper.createObjectNode();
            response.put("jsonrpc", "2.0");
            response.put("protocolVersion", PROTOCOL_VERSION);
            if (id != null) {
                response.put("id", id);
            }
            response.set("result", mapper.valueToTree(result));
            return mapper.writeValueAsString(response);
        } catch (Exception e) {
            return errorJson(mapper, id, -32000, e.getMessage());
        }
    }

    private static Object dispatch(String method, JsonNode params, RenameHandler renameHandler) throws Exception {
        if (method == null) {
            throw new IllegalArgumentException("missing method");
        }
        return switch (method) {
            case "rename" -> renameHandler.rename(params);
            default -> throw new IllegalArgumentException("unknown method: " + method);
        };
    }

    private static String errorJson(ObjectMapper mapper, Integer id, int code, String message) {
        try {
            ObjectNode err = mapper.createObjectNode();
            err.put("jsonrpc", "2.0");
            err.put("protocolVersion", PROTOCOL_VERSION);
            if (id != null) {
                err.put("id", id);
            }
            ObjectNode errorNode = mapper.createObjectNode();
            errorNode.put("code", code);
            errorNode.put("message", message != null ? message : "unknown error");
            err.set("error", errorNode);
            return mapper.writeValueAsString(err);
        } catch (Exception ignored) {
            return null;
        }
    }
}
