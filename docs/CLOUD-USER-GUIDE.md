# Cloud application user documentation

The cloud guide is separate from the desktop handbook.

- **In the application:** open `/help`. Contextual Help is also available in the shared project toolbar. The guide is public and searchable.
- **Complete standalone document:** [6eek cloud user guide](https://github.com/gen-ai-videoflow/videoflow-web/blob/main/static/help/user-guide.md).
- **Content source:** `src/lib/cloud/help/guide.json` in the companion frontend repository drives both the in-app guide and downloadable document.

The guide covers getting started, watching, projects, Story, Canvas and prompt templates, OpenCut editing, captions, downloading, publishing uploads and generated outputs, Premium and credits, payment methods, profile and security, creator analytics and troubleshooting.

To update it, edit the content source, then run:

```sh
node frontend/scripts/generate-user-guide.mjs
node frontend/scripts/generate-user-guide.mjs --check
```

In the separated frontend repository, run the same commands without the `frontend/` prefix. Keep unavailable capabilities clearly labelled; do not document planned services as active.

## Operator documentation

- [Deployment](CLOUD-DEPLOYMENT.md)
- [AI activation](AI-ACTIVATION.md)
- [Payment operations](CLOUD-PAYMENT-OPERATIONS.md)
- [CryptoRails](CRYPTORAILS.md)
- [Premium](PREMIUM.md)
- [Editor operations](EDITOR-OPERATIONS.md)
- [Request capacity](ops/REQUEST-CAPACITY.md)
- [Admin console](ops/ADMIN-CONSOLE.md)
- [Runtime model and pricing policy](RUNTIME-POLICY.md)

The existing [desktop handbook](handbook/README.md) includes local installation and provider configuration. It should not be used as the SaaS onboarding guide.
