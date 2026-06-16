package com.refute.openrewrite;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

/**
 * Pins the Java adapter to the shared golden wire-contract fixtures under
 * testdata/adapter-contracts/openrewrite/ (issue #76). The Go driver consumes
 * the same files in internal/backend/openrewrite/contract_test.go.
 */
class WireContractTest {
    private static final ObjectMapper MAPPER = new ObjectMapper();

    private static Path fixturesDir() {
        String dir = System.getProperty("contractFixtures");
        if (dir != null && !dir.isEmpty()) {
            return Path.of(dir);
        }
        // Fallback for an IDE run without the surefire system property.
        return Path.of("..", "..", "testdata", "adapter-contracts", "openrewrite");
    }

    private static JsonNode loadFixture(String name) throws IOException {
        return MAPPER.readTree(Files.readString(fixturesDir().resolve(name)));
    }

    @Test
    void methodRequestFixtureHasExpectedShape() throws IOException {
        JsonNode req = loadFixture("rename-method.request.json");
        assertEquals(Main.PROTOCOL_VERSION, req.get("protocolVersion").asInt());
        assertEquals("rename", req.get("method").asText());
        JsonNode params = req.get("params");
        assertEquals("hello", params.get("newName").asText());
        assertEquals("com.example.Greeter greet(..)", params.get("methodPattern").asText());
    }

    @Test
    void typeRequestFixtureHasExpectedShape() throws IOException {
        JsonNode req = loadFixture("rename-type.request.json");
        assertEquals(Main.PROTOCOL_VERSION, req.get("protocolVersion").asInt());
        JsonNode params = req.get("params");
        assertEquals("HelloService", params.get("newName").asText());
        assertEquals("com.example.Greeter", params.get("oldFullyQualifiedName").asText());
    }

    @Test
    void missingPatternProducesGoldenErrorEnvelope() throws IOException {
        // A rename request whose params lack both methodPattern and
        // oldFullyQualifiedName drives RenameHandler to the documented error.
        String request = "{\"jsonrpc\":\"2.0\",\"protocolVersion\":1,\"id\":1,\"method\":\"rename\","
                + "\"params\":{\"workspaceRoot\":\"/workspace\",\"newName\":\"hello\"}}";

        String produced = Main.handleLine(request, MAPPER, new RenameHandler(MAPPER));

        assertEquals(loadFixture("error.response.json"), MAPPER.readTree(produced));
    }

    @Test
    void methodRenameProducesGoldenSuccessEnvelope(@TempDir Path workspace) throws IOException {
        // Materialize the fixture scenario: com.example.Greeter with a greet
        // method, renamed to hello. Renaming the declaration (no caller) changes
        // exactly one file, matching the single-entry golden success response.
        Path javaFile = workspace.resolve("src/main/java/com/example/Greeter.java");
        Files.createDirectories(javaFile.getParent());
        Files.writeString(javaFile,
                "package com.example;\n\npublic class Greeter {\n    public String greet(String name) {\n"
                        + "        return \"Hello, \" + name + \"!\";\n    }\n}\n");

        ObjectNode params = MAPPER.createObjectNode();
        params.put("workspaceRoot", workspace.toString());
        params.put("newName", "hello");
        params.put("methodPattern", "com.example.Greeter greet(..)");
        ObjectNode request = MAPPER.createObjectNode();
        request.put("jsonrpc", "2.0");
        request.put("protocolVersion", Main.PROTOCOL_VERSION);
        request.put("id", 1);
        request.put("method", "rename");
        request.set("params", params);

        String produced = Main.handleLine(MAPPER.writeValueAsString(request), MAPPER, new RenameHandler(MAPPER));
        // Normalize the temp workspace path back to the fixture placeholder.
        String normalized = produced.replace(workspace.toAbsolutePath().normalize().toString(), "/workspace");

        assertEquals(loadFixture("rename.response.json"), MAPPER.readTree(normalized));
    }

    @Test
    void skewedProtocolVersionIsRejectedNotExecuted() throws IOException {
        String request = "{\"jsonrpc\":\"2.0\",\"protocolVersion\":999,\"id\":1,\"method\":\"rename\","
                + "\"params\":{\"workspaceRoot\":\"/workspace\",\"newName\":\"hello\","
                + "\"methodPattern\":\"com.example.Greeter greet(..)\"}}";

        JsonNode response = MAPPER.readTree(Main.handleLine(request, MAPPER, new RenameHandler(MAPPER)));

        assertEquals(Main.PROTOCOL_VERSION, response.get("protocolVersion").asInt());
        assertEquals(-32600, response.get("error").get("code").asInt());
        assertTrue(response.get("error").get("message").asText().contains("unsupported protocol version"));
    }
}
