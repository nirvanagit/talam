#!/usr/bin/env python3
"""One-shot migration of docs/ (the repo's markdown documentation graph) into
site/content/en/docs/ (Hugo/Docsy content). Not meant to run repeatedly —
docs/ stays the source of truth for engineers reading the repo directly;
this site is the browsable, styled front end for everyone else. Re-run by
hand after a docs/ change and re-review the diff, same as any port.
"""
import os
import re
import sys

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
DOCS_DIR = os.path.join(REPO_ROOT, "docs")
CONTENT_DIR = os.path.join(REPO_ROOT, "site", "content", "en", "docs")

# source docs/-relative path -> (dest content/en/docs/-relative path, weight)
# Weight controls left-nav ordering within a section (Docsy convention).
MANIFEST = [
    ("README.md", "_index.md", 1),
    ("architecture/overview.md", "architecture/_index.md", 10),
    ("components/operator/README.md", "components/operator/_index.md", 10),
    ("components/agent/README.md", "components/agent/_index.md", 20),
    ("components/server/README.md", "components/server/_index.md", 30),
    ("components/mesh-mcp/README.md", "components/mesh-mcp/_index.md", 40),
    ("concepts/analyzer-interface.md", "concepts/analyzer-interface.md", 10),
    ("concepts/finding-and-incident.md", "concepts/finding-and-incident.md", 20),
    ("concepts/remediation-flow.md", "concepts/remediation-flow.md", 30),
    ("concepts/object-model.md", "concepts/object-model.md", 40),
    ("concepts/security-model.md", "concepts/security-model.md", 50),
    ("decisions/README.md", "decisions/_index.md", 10),
    ("decisions/0001-server-agent-operator-split.md", "decisions/0001-server-agent-operator-split.md", 10),
    ("decisions/0002-deterministic-analyzers-then-llm.md", "decisions/0002-deterministic-analyzers-then-llm.md", 20),
    ("decisions/0003-human-in-the-loop-remediation.md", "decisions/0003-human-in-the-loop-remediation.md", 30),
    ("decisions/0004-mesh-agnostic-analyzer-interface.md", "decisions/0004-mesh-agnostic-analyzer-interface.md", 40),
    ("decisions/0005-crd-native-incidents-and-resolutions.md", "decisions/0005-crd-native-incidents-and-resolutions.md", 50),
    ("decisions/0006-mcp-evidence-enrichment.md", "decisions/0006-mcp-evidence-enrichment.md", 60),
    ("api/crds.md", "api/crds.md", 10),
    ("api/analyzer-catalog.md", "api/analyzer-catalog.md", 20),
    ("glossary/README.md", "glossary/_index.md", 10),
]

# Section landing pages that need their own _index.md with just front matter
# (docs/ has no single file for these — they're implied by the folder).
SECTION_INDEXES = {
    "components": ("Components", "One node per deployable — what each piece does, owns, and depends on.", 20),
    "concepts": ("Concepts", "Cross-cutting ideas used throughout the rest of the docs.", 30),
    "api": ("API", "Contracts: CRD schemas and the analyzer catalog.", 40),
    "architecture": None,  # has its own overview.md as _index.md already
    "decisions": None,
    "glossary": None,
}

SRC_TO_DEST = {src: dest for src, dest, _ in MANIFEST}


def dest_url(src_rel):
    """docs/-relative source path -> absolute site URL, no .md, trailing slash."""
    dest_rel = SRC_TO_DEST.get(src_rel)
    if dest_rel is None:
        return None
    url = "/docs/" + dest_rel
    url = re.sub(r"_index\.md$", "", url)
    url = re.sub(r"\.md$", "/", url)
    return url


LINK_RE = re.compile(r"\[([^\]]+)\]\(([^)#\s]+)(#[^)\s]*)?\)")


def rewrite_links(text, src_rel):
    src_dir = os.path.dirname(src_rel)

    def repl(m):
        label, target, anchor = m.group(1), m.group(2), m.group(3) or ""
        if target.startswith("http://") or target.startswith("https://") or target.startswith("/"):
            return m.group(0)
        if not target.endswith(".md"):
            return m.group(0)  # non-doc relative link (rare) — leave as-is
        # Resolve target relative to the source file's directory, back to a
        # docs/-relative path, matching MANIFEST's keys.
        resolved = os.path.normpath(os.path.join(src_dir, target)).replace(os.sep, "/")
        new_url = dest_url(resolved)
        if new_url is None:
            print(f"  WARNING: {src_rel}: unresolved internal link -> {target} (resolved {resolved})", file=sys.stderr)
            return m.group(0)
        return f"[{label}]({new_url}{anchor})"

    return LINK_RE.sub(repl, text)


def extract_title(text):
    m = re.search(r"^#\s+(.+)$", text, re.MULTILINE)
    return m.group(1).strip() if m else "Untitled"


def strip_h1(text):
    return re.sub(r"^#\s+.+\n+", "", text, count=1, flags=re.MULTILINE)


def front_matter(title, weight, description=""):
    esc_title = title.replace('"', '\\"')
    lines = ["---", f'title: "{esc_title}"', f"weight: {weight}"]
    if description:
        esc_desc = description.replace('"', '\\"')
        lines.append(f'description: "{esc_desc}"')
    lines += ["---", ""]
    return "\n".join(lines)


def main():
    for src_rel, dest_rel, weight in MANIFEST:
        src_path = os.path.join(DOCS_DIR, src_rel)
        dest_path = os.path.join(CONTENT_DIR, dest_rel)
        os.makedirs(os.path.dirname(dest_path), exist_ok=True)

        with open(src_path, "r") as f:
            text = f.read()

        title = extract_title(text)
        body = strip_h1(text)
        body = rewrite_links(body, src_rel)
        out = front_matter(title, weight) + body
        with open(dest_path, "w") as f:
            f.write(out)
        print(f"  {src_rel} -> content/en/docs/{dest_rel}")

    for section, meta in SECTION_INDEXES.items():
        if meta is None:
            continue
        title, desc, weight = meta
        dest_path = os.path.join(CONTENT_DIR, section, "_index.md")
        os.makedirs(os.path.dirname(dest_path), exist_ok=True)
        with open(dest_path, "w") as f:
            f.write(front_matter(title, weight, desc))
        print(f"  (generated) content/en/docs/{section}/_index.md")


if __name__ == "__main__":
    main()
