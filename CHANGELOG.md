# Changelog

## [0.4.0](https://github.com/Matheus-Thurler/KubeSilicon/compare/v0.3.2...v0.4.0) (2026-11-11)


### Features

* **job:** run jobs in parallel when their memory fits ([05084d3](https://github.com/Matheus-Thurler/KubeSilicon/commit/05084d3355b8ebf798a404a720f13114c3cc9d3a))
* **job:** run jobs in parallel when their memory fits ([371ae9c](https://github.com/Matheus-Thurler/KubeSilicon/commit/371ae9c031637c48e2a99259686abcd2ce9dde65))


### Bug Fixes

* **job:** drop the stale memory sample on retry and the empty peak line ([52cd96c](https://github.com/Matheus-Thurler/KubeSilicon/commit/52cd96c2eedd04bfb6a754ce2b2293dee9f4350f))

## [0.3.2](https://github.com/Matheus-Thurler/KubeSilicon/compare/v0.3.1...v0.3.2) (2026-11-11)


### Bug Fixes

* close two ways a cluster object could reach files or code on the host ([bed01c0](https://github.com/Matheus-Thurler/KubeSilicon/commit/bed01c0b21d201325537a0419d83d932b45e17aa))
* **image:** reject disk sources that read files of the host ([796e6fb](https://github.com/Matheus-Thurler/KubeSilicon/commit/796e6fbba5e8706d2c26ec99b4b4183dcb85c058))
* **job:** validate python and dependencies before uv runs them ([77dcaf1](https://github.com/Matheus-Thurler/KubeSilicon/commit/77dcaf174da3952e63c760f38fa229c5416c8a65))

## [0.3.1](https://github.com/Matheus-Thurler/KubeSilicon/compare/v0.3.0...v0.3.1) (2026-11-10)


### Bug Fixes

* give every Fashion-MNIST run its own bucket prefix ([faab3f5](https://github.com/Matheus-Thurler/KubeSilicon/commit/faab3f5253e0eeeb2d8530fbb2e0f7755447f5b2))
* give every Fashion-MNIST run its own bucket prefix ([c80daab](https://github.com/Matheus-Thurler/KubeSilicon/commit/c80daabb390576ac19b26f00f4f10530d8757852))

## [0.3.0](https://github.com/Matheus-Thurler/KubeSilicon/compare/v0.2.0...v0.3.0) (2026-11-09)


### Features

* Fashion-MNIST pipeline with data, GPU training and evaluation ([5265fa0](https://github.com/Matheus-Thurler/KubeSilicon/commit/5265fa0a00c00388685c22e4ca322fbd54b12e94))
* Fashion-MNIST pipeline with data, GPU training and evaluation ([1271984](https://github.com/Matheus-Thurler/KubeSilicon/commit/12719846f3cd0ff4dbd82ea0e03b2dfe92e02752))

## [0.2.0](https://github.com/Matheus-Thurler/KubeSilicon/compare/v0.1.0...v0.2.0) (2026-11-09)


### Features

* initial import with SiliconVM lifecycle on Apple Silicon ([458f16e](https://github.com/Matheus-Thurler/KubeSilicon/commit/458f16e1fe2baa3ee69a2c506f64bb8f36bd5f33))
* Kubeflow Pipelines integration, status.logTail and kind-kubeflow setup script ([76c025b](https://github.com/Matheus-Thurler/KubeSilicon/commit/76c025b8f29b241c904de9d2008d1c16a0cb8f70))
* kubesilicon client with ip, ssh and serial console ([bbcca7a](https://github.com/Matheus-Thurler/KubeSilicon/commit/bbcca7ae042d341993491a131c37384471f895b0))
* make install, example VM and terminal tutorial tapes ([2875713](https://github.com/Matheus-Thurler/KubeSilicon/commit/2875713b72d17188f97ca2f5fc94a7dcfc4f8137))
* one launcher process per VM and DNS name per VM ([6d7cd70](https://github.com/Matheus-Thurler/KubeSilicon/commit/6d7cd708835ca891a36cddcf8c853221797f3c2d))
* orphan cleanup on start and a LaunchAgent installer ([8d7b144](https://github.com/Matheus-Thurler/KubeSilicon/commit/8d7b1445dbb326da1a1f976b5833cb91692d6ebf))
* production hardening for SiliconJob and the daemon ([68c58bd](https://github.com/Matheus-Thurler/KubeSilicon/commit/68c58bdb08834e23e5521003c6517cd021a0a83b))
* production hardening, retry on interruption, least-privilege daemon, pending timeout ([705a4f0](https://github.com/Matheus-Thurler/KubeSilicon/commit/705a4f09736283eb06a6aaee73774cec7a1515d9))
* SiliconJob, native GPU jobs on the Mac (MPS) with venv, sandbox, queue and artifacts ([8f5be85](https://github.com/Matheus-Thurler/KubeSilicon/commit/8f5be85238571f36ab36f440e23d0b4a79624c74))


### Bug Fixes

* cached disk mode to stop random guest crashes ([c9f9de4](https://github.com/Matheus-Thurler/KubeSilicon/commit/c9f9de4b131b5433bca4caabc3dddcabdd62ec48))
* never rerun a SiliconJob whose status is already final ([eaaf65a](https://github.com/Matheus-Thurler/KubeSilicon/commit/eaaf65a66b858c06f10167d0d00a6b25de7b79af))
* **vm:** keep the NIC MAC across boots ([a9bc9f7](https://github.com/Matheus-Thurler/KubeSilicon/commit/a9bc9f7512e6624e3c306e095aeb6723915b1186))
