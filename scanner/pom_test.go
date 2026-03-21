package scanner_test

import (
	"os"
	"path/filepath"
	"testing"

	"dep-health/scanner"
)

var pomScanner = &scanner.PomScanner{}

func writePom(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "pom.xml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing pom.xml: %v", err)
	}
	return path
}

func TestPomScanner_Matches(t *testing.T) {
	cases := []struct {
		path  string
		match bool
	}{
		{"pom.xml", true},
		{"/some/path/pom.xml", true},
		{"mypom.xml", false},
		{"build.gradle", false},
		{"package.json", false},
	}
	for _, tc := range cases {
		got := pomScanner.Matches(tc.path)
		if got != tc.match {
			t.Errorf("Matches(%q) = %v, want %v", tc.path, got, tc.match)
		}
	}
}

func TestPomScanner_BasicDependencies(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <dependencies>
    <dependency>
      <groupId>org.springframework</groupId>
      <artifactId>spring-core</artifactId>
      <version>5.3.23</version>
    </dependency>
    <dependency>
      <groupId>com.fasterxml.jackson.core</groupId>
      <artifactId>jackson-databind</artifactId>
      <version>2.13.0</version>
    </dependency>
  </dependencies>
</project>`
	path := writePom(t, content)
	deps, err := pomScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d: %v", len(deps), deps)
	}
	byName := map[string]string{}
	for _, d := range deps {
		byName[d.Name] = d.CurrentVersion
		if d.Ecosystem != "maven" {
			t.Errorf("%s: ecosystem = %q, want 'maven'", d.Name, d.Ecosystem)
		}
	}
	if byName["org.springframework:spring-core"] != "5.3.23" {
		t.Errorf("spring-core version = %q", byName["org.springframework:spring-core"])
	}
	if byName["com.fasterxml.jackson.core:jackson-databind"] != "2.13.0" {
		t.Errorf("jackson-databind version = %q", byName["com.fasterxml.jackson.core:jackson-databind"])
	}
}

func TestPomScanner_PropertyResolution(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <properties>
    <jackson.version>2.13.0</jackson.version>
    <guava.version>30.1.1-jre</guava.version>
  </properties>
  <dependencies>
    <dependency>
      <groupId>com.fasterxml.jackson.core</groupId>
      <artifactId>jackson-databind</artifactId>
      <version>${jackson.version}</version>
    </dependency>
    <dependency>
      <groupId>com.google.guava</groupId>
      <artifactId>guava</artifactId>
      <version>${guava.version}</version>
    </dependency>
  </dependencies>
</project>`
	path := writePom(t, content)
	deps, err := pomScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d: %v", len(deps), deps)
	}
	byName := map[string]string{}
	for _, d := range deps {
		byName[d.Name] = d.CurrentVersion
	}
	if byName["com.fasterxml.jackson.core:jackson-databind"] != "2.13.0" {
		t.Errorf("jackson-databind: property not resolved, got %q", byName["com.fasterxml.jackson.core:jackson-databind"])
	}
	if byName["com.google.guava:guava"] != "30.1.1-jre" {
		t.Errorf("guava: property not resolved, got %q", byName["com.google.guava:guava"])
	}
}

func TestPomScanner_SystemScopeSkipped(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <dependencies>
    <dependency>
      <groupId>org.springframework</groupId>
      <artifactId>spring-core</artifactId>
      <version>5.3.23</version>
    </dependency>
    <dependency>
      <groupId>com.example</groupId>
      <artifactId>internal-sdk</artifactId>
      <version>1.0.0</version>
      <scope>system</scope>
      <systemPath>/lib/internal.jar</systemPath>
    </dependency>
  </dependencies>
</project>`
	path := writePom(t, content)
	deps, err := pomScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep (system scope skipped), got %d: %v", len(deps), deps)
	}
	if deps[0].Name != "org.springframework:spring-core" {
		t.Errorf("unexpected dep: %q", deps[0].Name)
	}
}

func TestPomScanner_TestScopeIncluded(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <dependencies>
    <dependency>
      <groupId>junit</groupId>
      <artifactId>junit</artifactId>
      <version>4.13.1</version>
      <scope>test</scope>
    </dependency>
  </dependencies>
</project>`
	path := writePom(t, content)
	deps, err := pomScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected test-scoped dep to be included, got %d deps", len(deps))
	}
}

func TestPomScanner_ParentIncluded(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <parent>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-parent</artifactId>
    <version>2.6.0</version>
  </parent>
  <dependencies>
    <dependency>
      <groupId>com.google.guava</groupId>
      <artifactId>guava</artifactId>
      <version>30.1.1-jre</version>
    </dependency>
  </dependencies>
</project>`
	path := writePom(t, content)
	deps, err := pomScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps (parent + guava), got %d: %v", len(deps), deps)
	}
	byName := map[string]string{}
	for _, d := range deps {
		byName[d.Name] = d.CurrentVersion
	}
	if byName["org.springframework.boot:spring-boot-starter-parent"] != "2.6.0" {
		t.Errorf("parent not included or wrong version: %v", byName)
	}
}

func TestPomScanner_DependencyManagement(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <groupId>org.springframework.cloud</groupId>
        <artifactId>spring-cloud-dependencies</artifactId>
        <version>2021.0.0</version>
        <type>pom</type>
        <scope>import</scope>
      </dependency>
    </dependencies>
  </dependencyManagement>
</project>`
	path := writePom(t, content)
	deps, err := pomScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 BOM dep from dependencyManagement, got %d", len(deps))
	}
	if deps[0].Name != "org.springframework.cloud:spring-cloud-dependencies" {
		t.Errorf("name = %q", deps[0].Name)
	}
	if deps[0].CurrentVersion != "2021.0.0" {
		t.Errorf("version = %q", deps[0].CurrentVersion)
	}
}

func TestPomScanner_MissingVersionSkipped(t *testing.T) {
	// Deps without a version are inherited from the parent BOM — we can't
	// report a current version so they must be skipped.
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <dependencies>
    <dependency>
      <groupId>org.springframework.boot</groupId>
      <artifactId>spring-boot-starter-web</artifactId>
      <!-- version inherited from parent BOM -->
    </dependency>
    <dependency>
      <groupId>com.google.guava</groupId>
      <artifactId>guava</artifactId>
      <version>30.1.1-jre</version>
    </dependency>
  </dependencies>
</project>`
	path := writePom(t, content)
	deps, err := pomScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep (no-version skipped), got %d: %v", len(deps), deps)
	}
	if deps[0].Name != "com.google.guava:guava" {
		t.Errorf("name = %q, want 'com.google.guava:guava'", deps[0].Name)
	}
}

func TestPomScanner_UnresolvablePropertySkipped(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <dependencies>
    <dependency>
      <groupId>com.example</groupId>
      <artifactId>something</artifactId>
      <version>${undefined.property}</version>
    </dependency>
    <dependency>
      <groupId>com.google.guava</groupId>
      <artifactId>guava</artifactId>
      <version>30.1.1-jre</version>
    </dependency>
  </dependencies>
</project>`
	path := writePom(t, content)
	deps, err := pomScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep (unresolvable property skipped), got %d: %v", len(deps), deps)
	}
}

func TestPomScanner_NameFormat(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <dependencies>
    <dependency>
      <groupId>org.apache.logging.log4j</groupId>
      <artifactId>log4j-core</artifactId>
      <version>2.14.1</version>
    </dependency>
  </dependencies>
</project>`
	path := writePom(t, content)
	deps, err := pomScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep")
	}
	// Name must be groupId:artifactId
	if deps[0].Name != "org.apache.logging.log4j:log4j-core" {
		t.Errorf("name = %q, want 'org.apache.logging.log4j:log4j-core'", deps[0].Name)
	}
}

func TestPomScanner_RepoURL(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <dependencies>
    <dependency>
      <groupId>com.google.guava</groupId>
      <artifactId>guava</artifactId>
      <version>30.1.1-jre</version>
    </dependency>
  </dependencies>
</project>`
	path := writePom(t, content)
	deps, err := pomScanner.Parse(path, "https://gitlab.company.com/org/repo")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) == 0 {
		t.Fatal("expected 1 dep")
	}
	if deps[0].RepoURL != "https://gitlab.company.com/org/repo" {
		t.Errorf("RepoURL = %q", deps[0].RepoURL)
	}
}

func TestPomScanner_Java8Properties(t *testing.T) {
	// Ensures a pom.xml with java.version=1.8 is parsed without issue.
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <properties>
    <java.version>1.8</java.version>
    <maven.compiler.source>1.8</maven.compiler.source>
    <maven.compiler.target>1.8</maven.compiler.target>
    <spring.version>5.3.23</spring.version>
  </properties>
  <dependencies>
    <dependency>
      <groupId>org.springframework</groupId>
      <artifactId>spring-core</artifactId>
      <version>${spring.version}</version>
    </dependency>
  </dependencies>
</project>`
	path := writePom(t, content)
	deps, err := pomScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error on Java 8 pom: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].CurrentVersion != "5.3.23" {
		t.Errorf("version = %q, want '5.3.23'", deps[0].CurrentVersion)
	}
}

func TestPomScanner_Empty(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <groupId>com.example</groupId>
  <artifactId>empty-project</artifactId>
  <version>1.0.0</version>
</project>`
	path := writePom(t, content)
	deps, err := pomScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 deps, got %d", len(deps))
	}
}
