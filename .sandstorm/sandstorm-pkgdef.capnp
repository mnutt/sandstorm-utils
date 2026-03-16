@0x89f4ed93c54e7ea1;

using Spk = import "/sandstorm/package.capnp";

const pkgdef :Spk.PackageDefinition = (
  # Replace this with the package ID for the key stored in your Sandstorm
  # signing keyring secret before relying on release builds.
  id = "fuzmnvev1qe24v8wz0srwm9cg0f6m31r9ze7ut9k0zxcfqvvrjdh",

  manifest = (
    appTitle = (defaultText = "sandstorm-utils testapp"),
    appVersion = 1,
    appMarketingVersion = (defaultText = "0.1.0"),

    actions = [
      (
        title = (defaultText = "New Integration Harness"),
        command = .command
      )
    ],

    continueCommand = .command,

    metadata = (
      website = "https://github.com/mnutt/sandstorm-utils",
      codeUrl = "https://github.com/mnutt/sandstorm-utils",
      license = (openSource = apache2),
      categories = [developerTools],
      author = (
        contactEmail = "michael@nutt.im"
      ),
      description = (defaultText = embed "description.md"),
      shortDescription = (defaultText = "Integration harness for sandstorm-utils"),
    ),
  ),

  sourceMap = (
    searchPath = [
      ( sourcePath = "." ),
      (
        sourcePath = "/",
        hidePaths = [ "home", "proc", "sys", "run",
                      "etc/passwd", "etc/hosts", "etc/host.conf",
                      "etc/nsswitch.conf", "etc/resolv.conf" ]
      )
    ]
  ),

  fileList = "sandstorm-files.list",
  alwaysInclude = [
    "opt/app/testapp",
    "sandstorm-http-bridge",
  ],

  bridgeConfig = (
    apiPath = "/",
    viewInfo = (
      eventTypes = [
        (
          name = "testActivity",
          verbPhrase = (defaultText = "ran test activity"),
          description = (defaultText = "Activity event emitted by the sandstorm-utils test harness."),
          requiredPermission = (everyone = void),
          notifySubscribers = false,
        )
      ]
    )
  )
);

const command :Spk.Manifest.Command = (
  argv = ["/sandstorm-http-bridge", "3000", "--", "/opt/app/testapp/bin/testapp-server"],
  environ = [
    (key = "PATH", value = "/opt/app/testapp/bin:/usr/local/bin:/usr/bin:/bin"),
    (key = "PORT", value = "3000"),
    (key = "HOME", value = "/var"),
    (key = "SANDSTORM", value = "1"),
    (key = "TMPDIR", value = "/tmp"),
  ]
);
