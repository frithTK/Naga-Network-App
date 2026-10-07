String? loadControlPlaneToken({Map<String, String>? environment}) {
  // Web builds must not embed a bearer token into the public JavaScript bundle.
  // A web caller can provide an explicit token to ControlPlaneClient when its
  // delivery mechanism is controlled outside the bundle.
  return null;
}
