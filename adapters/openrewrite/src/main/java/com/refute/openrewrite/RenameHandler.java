package com.refute.openrewrite;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.openrewrite.InMemoryExecutionContext;
import org.openrewrite.Recipe;
import org.openrewrite.Result;
import org.openrewrite.SourceFile;
import org.openrewrite.internal.InMemoryLargeSourceSet;
import org.openrewrite.java.ChangeMethodName;
import org.openrewrite.java.ChangeType;
import org.openrewrite.java.JavaParser;

import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;
import java.util.stream.Stream;

/**
 * Handles the "rename" JSON-RPC method using OpenRewrite recipes.
 *
 * The caller must provide either:
 *   - methodPattern + newName  → ChangeMethodName (method rename)
 *   - oldFullyQualifiedName + newName  → ChangeType (class/type rename)
 *
 * All .java files under workspaceRoot (excluding target/ and .git/) are
 * parsed and passed to the recipe. Changed files are returned as
 * { path, newContent } entries.
 */
public class RenameHandler {

    private final ObjectMapper mapper;

    public RenameHandler(ObjectMapper mapper) {
        this.mapper = mapper;
    }

    public List<Map<String, String>> rename(JsonNode params) throws Exception {
        String workspaceRoot = requiredString(params, "workspaceRoot");
        String newName = requiredString(params, "newName");

        Path baseDir = Paths.get(workspaceRoot).toAbsolutePath();
        List<Path> javaFiles = collectJavaFiles(baseDir);

        if (javaFiles.isEmpty()) {
            return List.of();
        }

        JavaParser parser = JavaParser.fromJavaVersion().build();
        InMemoryExecutionContext ctx = new InMemoryExecutionContext(e -> System.err.println("parse error: " + e.getMessage()));

        List<SourceFile> sources;
        try (Stream<SourceFile> stream = parser.parse(javaFiles, baseDir, ctx)) {
            sources = new ArrayList<>(stream.collect(Collectors.toList()));
        }

        Recipe recipe = buildRecipe(params, newName);
        List<Result> results = recipe.run(new InMemoryLargeSourceSet(sources), ctx).getChangeset().getAllResults();

        return results.stream()
                .filter(r -> r.getAfter() != null)
                .map(r -> Map.of(
                        "path", baseDir.resolve(r.getAfter().getSourcePath()).toAbsolutePath().normalize().toString(),
                        "newContent", r.getAfter().printAll()
                ))
                .collect(Collectors.toList());
    }

    private Recipe buildRecipe(JsonNode params, String newName) {
        if (params.has("methodPattern")) {
            String methodPattern = params.get("methodPattern").asText();
            return new ChangeMethodName(methodPattern, newName, null, false);
        }
        if (params.has("oldFullyQualifiedName")) {
            String oldFqn = params.get("oldFullyQualifiedName").asText();
            String packagePrefix = oldFqn.contains(".")
                    ? oldFqn.substring(0, oldFqn.lastIndexOf('.') + 1)
                    : "";
            String newFqn = packagePrefix + newName;
            return new ChangeType(oldFqn, newFqn, false);
        }
        throw new IllegalArgumentException("params must include either 'methodPattern' or 'oldFullyQualifiedName'");
    }

    private List<Path> collectJavaFiles(Path baseDir) throws Exception {
        try (Stream<Path> walk = Files.walk(baseDir)) {
            return walk
                    .filter(Files::isRegularFile)
                    .filter(p -> p.toString().endsWith(".java"))
                    .filter(p -> !isExcluded(baseDir, p))
                    .sorted()
                    .collect(Collectors.toList());
        }
    }

    private boolean isExcluded(Path baseDir, Path path) {
        Path relative = baseDir.relativize(path);
        String rel = relative.toString();
        return rel.startsWith("target" + java.io.File.separator)
                || rel.startsWith(".git" + java.io.File.separator)
                || rel.contains(java.io.File.separator + "target" + java.io.File.separator);
    }

    private String requiredString(JsonNode params, String key) {
        if (params == null || !params.has(key) || params.get(key).isNull()) {
            throw new IllegalArgumentException("missing required param: " + key);
        }
        return params.get(key).asText();
    }
}
