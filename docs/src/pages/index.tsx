import { useState } from 'react';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';
import CodeBlock from '@theme/CodeBlock';

function CopyableCommand({ command }: { command: string }) {
    const [copied, setCopied] = useState(false);

    const handleCopy = async () => {
        await navigator.clipboard.writeText(command);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
    };

    return (
        <div className="copyable-command">
            <code>{command}</code>
            <button
                className="copy-button"
                onClick={handleCopy}
                aria-label={copied ? 'Copied!' : 'Copy to clipboard'}
                title={copied ? 'Copied!' : 'Copy to clipboard'}>
                {copied ? (
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <polyline points="20 6 9 17 4 12" />
                    </svg>
                ) : (
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                        <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                    </svg>
                )}
            </button>
        </div>
    );
}

type FeatureItem = {
    title: string;
    description: JSX.Element;
    icon: string;
};

const FeatureList: FeatureItem[] = [
    {
        title: 'Stream Without Downloading',
        icon: '01',
        description: (
            <>
                List and read individual files from OCI images without downloading the entire
                layer. Powered by{' '}
                <Link href="https://github.com/containerd/stargz-snapshotter">eStargz</Link>.
            </>
        ),
    },
    {
        title: 'Standard OCI',
        icon: '02',
        description: (
            <>
                Works with any OCI-compliant registry (GHCR, Docker Hub, ECR, GCR). No special
                server-side software required.
            </>
        ),
    },
    {
        title: 'Go Library & CLI',
        icon: '03',
        description: (
            <>
                Use it as a standalone CLI tool for scripts or embed it directly into your Go
                applications as a library.
            </>
        ),
    },
];

function Feature({ title, description, icon }: FeatureItem) {
    return (
        <div className="feature-card">
            <div className="feature-icon" aria-hidden="true">
                {icon}
            </div>
            <Heading as="h3">{title}</Heading>
            <p>{description}</p>
        </div>
    );
}

function HomepageHeader() {
    const { siteConfig } = useDocusaurusContext();
    return (
        <header className="blobber-hero">
            <div className="container hero-grid">
                <div className="hero-copy">
                    <p className="hero-eyebrow">Meigma open source</p>
                    <Heading as="h1" className="hero-title">
                        Store files in OCI registries, stream instantly.
                    </Heading>
                    <p className="hero-subtitle">
                        {siteConfig.tagline}. Stream the exact files you need without pulling
                        entire layers.
                    </p>
                    <div className="hero-pills">
                        <span className="hero-pill">OCI native</span>
                        <span className="hero-pill">eStargz indexing</span>
                        <span className="hero-pill">CLI + Go SDK</span>
                    </div>
                    <div className="hero-ctas">
                        <Link
                            className="button button--lg hero-cta"
                            to="/docs/getting-started/cli/installation">
                            Get Started
                        </Link>
                        <Link
                            className="button button--lg button--outline hero-cta-outline"
                            to="https://github.com/meigma/blobber">
                            GitHub
                        </Link>
                    </div>
                    <div className="hero-install">
                        <div className="hero-install__label">Install in one line</div>
                        <div className="hero-install__commands">
                            <CopyableCommand command="curl -fsSL https://blobber.meigma.dev/install.sh | sh" />
                            <span>or</span>
                            <CopyableCommand command="brew install meigma/tap/blobber" />
                        </div>
                    </div>
                </div>
                <div className="hero-visual">
                    <div className="hero-card">
                        <div className="hero-card__title">Registry view</div>
                        <ul className="hero-tree">
                            <li>
                                <span className="hero-tree__dot" />
                                ghcr.io/myorg/config:v1
                            </li>
                            <li className="hero-tree__indent">app.yaml</li>
                            <li className="hero-tree__indent">routes.json</li>
                            <li className="hero-tree__indent">secrets/</li>
                        </ul>
                        <div className="hero-output">Indexed via eStargz in milliseconds</div>
                    </div>
                    <div className="hero-card">
                        <div className="hero-card__title">Stream a single file</div>
                        <div className="hero-command">
                            $ blobber cat ghcr.io/myorg/config:v1 app.yaml
                        </div>
                        <div className="hero-output">app.yaml (3.2 KB) streamed on demand</div>
                    </div>
                </div>
            </div>
        </header>
    );
}

function CodePreview() {
    return (
        <section className="blobber-section blobber-section--deep code-section">
            <div className="container">
                <Heading as="h2" className="section-heading">
                    Simple and powerful
                </Heading>
                <p className="section-lead">
                    Use Blobber as a CLI for scripts or as a Go library in your services. The API
                    is tiny, the behavior is predictable, and it works anywhere an OCI registry
                    does.
                </p>
                <div className="code-panel">
                    <Tabs>
                        <TabItem value="cli" label="CLI" default>
                            <CodeBlock language="bash">
                                {`# Push a directory to a registry
blobber push ./config ghcr.io/myorg/config:v1

# List files without downloading
blobber list ghcr.io/myorg/config:v1

# Stream a single file to stdout
blobber cat ghcr.io/myorg/config:v1 app.yaml`}
                            </CodeBlock>
                        </TabItem>
                        <TabItem value="go" label="Go Library">
                            <CodeBlock language="go">
                                {`package main

import (
    "context"
    "os"
    "github.com/meigma/blobber"
)

func main() {
    ctx := context.Background()
    client, _ := blobber.NewClient()

    // Push a directory
    client.Push(ctx, "ghcr.io/myorg/config:v1", os.DirFS("./config"))

    // List files without downloading
    img, _ := client.OpenImage(ctx, "ghcr.io/myorg/config:v1")
    entries, _ := img.List()
}`}
                            </CodeBlock>
                        </TabItem>
                    </Tabs>
                </div>
            </div>
        </section>
    );
}

function FeatureSection() {
    return (
        <section className="blobber-section blobber-section--muted">
            <div className="container">
                <Heading as="h2" className="section-heading">
                    Built for modern registry workflows
                </Heading>
                <p className="section-lead">
                    Blobber makes OCI registries behave like a file store while keeping the full
                    benefits of existing auth, caching, and immutability.
                </p>
                <div className="feature-grid">
                    {FeatureList.map((props, idx) => (
                        <Feature key={idx} {...props} />
                    ))}
                </div>
            </div>
        </section>
    );
}

function HighlightSection() {
    return (
        <section className="blobber-section">
            <div className="container split-grid">
                <div>
                    <Heading as="h2" className="section-heading">
                        List, inspect, and stream with eStargz
                    </Heading>
                    <p className="section-lead">
                        eStargz keeps a table of contents inside the image so Blobber can list
                        files instantly and stream a single path on demand.
                    </p>
                    <ul className="checklist">
                        <li>Inspect large images without pulling layers.</li>
                        <li>Stream a single file for config or secrets.</li>
                        <li>Pin by digest for immutable, reproducible builds.</li>
                    </ul>
                    <Link to="/docs/explanation/about-estargz">Learn how it works</Link>
                </div>
                <div className="highlight-card">
                    <div className="hero-card__title">Indexed file tree</div>
                    <div className="file-tree">
                        <div className="file-tree__item">
                            <span />
                            /config
                        </div>
                        <div className="file-tree__item file-tree__item--indent">
                            <span />
                            app.yaml
                        </div>
                        <div className="file-tree__item file-tree__item--indent">
                            <span />
                            routes.json
                        </div>
                        <div className="file-tree__item file-tree__item--indent">
                            <span />
                            policy.rego
                        </div>
                    </div>
                </div>
            </div>
        </section>
    );
}

function CompatibilitySection() {
    return (
        <section className="blobber-section blobber-section--muted">
            <div className="container">
                <Heading as="h2" className="section-heading">
                    Works with the registries you already use
                </Heading>
                <p className="section-lead">
                    Blobber relies on standard OCI APIs, so there is no server-side setup or
                    special infrastructure to deploy.
                </p>
                <div className="pill-row">
                    <span className="pill">GitHub Container Registry</span>
                    <span className="pill">Docker Hub</span>
                    <span className="pill">Amazon ECR</span>
                    <span className="pill">Google GCR</span>
                </div>
            </div>
        </section>
    );
}

function CTASection() {
    return (
        <section className="cta-section">
            <div className="container">
                <div className="cta-card">
                    <div>
                        <Heading as="h2">Ship files like images.</Heading>
                        <p>
                            Start with the CLI or embed the Go client. Blobber stays small and
                            predictable while giving you registry-native storage.
                        </p>
                    </div>
                    <div className="cta-actions">
                        <Link
                            className="button button--lg hero-cta"
                            to="/docs/getting-started/cli/installation">
                            Install Blobber
                        </Link>
                        <Link
                            className="button button--lg button--outline hero-cta-outline"
                            to="/docs">
                            Read the docs
                        </Link>
                    </div>
                </div>
            </div>
        </section>
    );
}

export default function Home(): JSX.Element {
    const { siteConfig } = useDocusaurusContext();
    return (
        <Layout
            title={`${siteConfig.title}`}
            description="Push and pull files to OCI container registries">
            <HomepageHeader />
            <main>
                <FeatureSection />
                <HighlightSection />
                <CompatibilitySection />
                <CodePreview />
                <CTASection />
            </main>
        </Layout>
    );
}
