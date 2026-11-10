# FQL Flutter example

This is an emulator-only runnable Flutter web application. It uses synthetic Firebase configuration for `demo-fql`, anonymous emulator authentication, and generated ownership rules. It does not connect to production Firebase.

From the repository root, install development prerequisites described in the root README, then:

```sh
make setup-integration
make generate-check
npx firebase emulators:start --project demo-fql --only firestore,auth --config firebase.json
```

In another terminal:

```sh
cd examples/flutter
flutter run -d web-server --web-port 7357
```

Open the printed local URL in your browser. Save a profile, create messages, tap a message to update it, and delete messages using the delete button. The list is scoped to the authenticated owner and exposes pending/cache status. Errors are shown in the screen.

Schemas live separately in `fQL/schemas/users.fQL` and `messages.fQL`. To change them, run `make generate` from the repository root and review both client and Firebase artifacts. `make integration-test` runs automated headless Chromium tests; no manual browser interaction is needed.

The app uses `127.0.0.1` emulator addresses and is configured for desktop browsers. Physical devices require deliberate host/network configuration and native project setup. A web build or emulator run does not establish Android/iOS acceptance.
