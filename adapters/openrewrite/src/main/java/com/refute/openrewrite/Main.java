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
 * line to stdout. Stderr is available for diagnostic logging.
 *
 * Supported methods: rename
 */
public class Main {
    public static void main(String[] args) throws Exception {
        ObjectMapper mapper = new ObjectMapper();
        RenameHandler renameHandler = new RenameHandler(mapper);
        BufferedReader reader = new BufferedReader(new InputStreamReader(System.in));
        PrintWriter writer = new PrintWriter(System.out, true);

        String line;
        while ((line = reader.readLine()) != null) {
            line = line.trim();
            if (line.isEmpty()) {
                continue;
            }

            JsonNode req;
            try {
                req = mapper.readTree(line);
            } catch (Exception e) {
                writeError(writer, mapper, null, -32700, "Parse error: " + e.getMessage());
                continue;
            }

            JsonNode idNode = req.get("id");
            Integer id = idNode != null && !idNode.isNull() ? idNode.asInt() : null;
            String method = req.has("method") ? req.get("method").asText() : null;
            JsonNode params = req.get("params");

            try {
                Object result = dispatch(method, params, renameHandler);
                ObjectNode response = mapper.createObjectNode();
                response.put("jsonrpc", "2.0");
                if (id != null) {
                    response.put("id", id);
                }
                response.set("result", mapper.valueToTree(result));
                writer.println(mapper.writeValueAsString(response));
            } catch (Exception e) {
                writeError(writer, mapper, id, -32000, e.getMessage());
            }
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

    private static void writeError(PrintWriter writer, ObjectMapper mapper, Integer id, int code, String message) {
        try {
            ObjectNode err = mapper.createObjectNode();
            err.put("jsonrpc", "2.0");
            if (id != null) {
                err.put("id", id);
            }
            ObjectNode errorNode = mapper.createObjectNode();
            errorNode.put("code", code);
            errorNode.put("message", message != null ? message : "unknown error");
            err.set("error", errorNode);
            writer.println(mapper.writeValueAsString(err));
        } catch (Exception ignored) {
        }
    }
}
