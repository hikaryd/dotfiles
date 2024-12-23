{ lib
, buildGoModule
, fetchFromGitHub
, src
, pam
, libX11
, pkg-config
, makeWrapper
}:

buildGoModule rec {
  pname = "emptty";
  version = "0.12.0";

  inherit src;

  vendorHash = "sha256-KGvBOFxFILHEfQFGlGmEe9QxhxPtpC0WxuUVzXxBtpQ=";

  nativeBuildInputs = [ pkg-config makeWrapper ];
  buildInputs = [ pam libX11 ];

  ldflags = [ "-s" "-w" ];

  postInstall = ''
    mkdir -p $out/etc/pam.d
    cp res/pam $out/etc/pam.d/emptty
  '';

  meta = with lib; {
    description = "Dead simple CLI Display Manager on TTY";
    homepage = "https://github.com/tvrzna/emptty";
    license = licenses.mit;
    maintainers = with maintainers; [ ];
    platforms = platforms.linux;
  };
}
