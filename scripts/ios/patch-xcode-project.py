#!/usr/bin/env python3
"""Idempotent inserts for PacketTunnel into the Flutter-generated pbxproj."""

from pathlib import Path

PBX = Path(__file__).resolve().parents[2] / "app/ios/Runner.xcodeproj/project.pbxproj"

IDS = {
    "vpn_swift_ref": "A10000000000000100000001",
    "vpn_swift_build": "A10000000000000100000002",
    "ctl_swift_ref": "A10000000000000100000003",
    "ctl_swift_build": "A10000000000000100000004",
    "ent_runner_ref": "A10000000000000100000005",
    "privacy_ref": "A10000000000000100000006",
    "privacy_build": "A10000000000000100000007",
    "pt_swift_ref": "A10000000000000100000008",
    "pt_swift_build": "A10000000000000100000009",
    "pt_plist_ref": "A1000000000000010000000A",
    "pt_ent_ref": "A1000000000000010000000B",
    "pt_product": "A1000000000000010000000C",
    "pt_group": "A1000000000000010000000D",
    "pt_target": "A1000000000000010000000E",
    "pt_sources": "A1000000000000010000000F",
    "pt_frameworks": "A10000000000000100000010",
    "pt_resources": "A10000000000000100000011",
    "embed_ext": "A10000000000000100000012",
    "embed_ext_file": "A10000000000000100000013",
    "pt_proxy": "A10000000000000100000014",
    "pt_dep": "A10000000000000100000015",
    "pt_debug": "A10000000000000100000016",
    "pt_release": "A10000000000000100000017",
    "pt_profile": "A10000000000000100000018",
    "pt_configs": "A10000000000000100000019",
}


def once(text: str, marker: str, insert: str) -> str:
    if marker in text:
        return text
    return text.replace(insert, insert + marker, 1)


def main() -> None:
    text = PBX.read_text()
    if IDS["pt_target"] in text:
        print("already patched")
        return

    text = text.replace(
        "/* End PBXBuildFile section */",
        f"""\t\t{IDS["vpn_swift_build"]} /* NagaVpnPlugin.swift in Sources */ = {{isa = PBXBuildFile; fileRef = {IDS["vpn_swift_ref"]} /* NagaVpnPlugin.swift */; }};
		{IDS["ctl_swift_build"]} /* NagaControl.swift in Sources */ = {{isa = PBXBuildFile; fileRef = {IDS["ctl_swift_ref"]} /* NagaControl.swift */; }};
		{IDS["privacy_build"]} /* PrivacyInfo.xcprivacy in Resources */ = {{isa = PBXBuildFile; fileRef = {IDS["privacy_ref"]} /* PrivacyInfo.xcprivacy */; }};
		{IDS["pt_swift_build"]} /* PacketTunnelProvider.swift in Sources */ = {{isa = PBXBuildFile; fileRef = {IDS["pt_swift_ref"]} /* PacketTunnelProvider.swift */; }};
		{IDS["embed_ext_file"]} /* PacketTunnel.appex in Embed App Extensions */ = {{isa = PBXBuildFile; fileRef = {IDS["pt_product"]} /* PacketTunnel.appex */; settings = {{ATTRIBUTES = (RemoveHeadersOnCopy, ); }}; }};
/* End PBXBuildFile section */""",
    )

    text = text.replace(
        "/* End PBXContainerItemProxy section */",
        f"""\t\t{IDS["pt_proxy"]} /* PBXContainerItemProxy */ = {{
			isa = PBXContainerItemProxy;
			containerPortal = 97C146E61CF9000F007C117D /* Project object */;
			proxyType = 1;
			remoteGlobalIDString = {IDS["pt_target"]};
			remoteInfo = PacketTunnel;
		}};
/* End PBXContainerItemProxy section */""",
    )

    text = text.replace(
        "/* End PBXCopyFilesBuildPhase section */",
        f"""\t\t{IDS["embed_ext"]} /* Embed App Extensions */ = {{
			isa = PBXCopyFilesBuildPhase;
			buildActionMask = 2147483647;
			dstPath = "";
			dstSubfolderSpec = 13;
			files = (
				{IDS["embed_ext_file"]} /* PacketTunnel.appex in Embed App Extensions */,
			);
			name = "Embed App Extensions";
			runOnlyForDeploymentPostprocessing = 0;
		}};
/* End PBXCopyFilesBuildPhase section */""",
    )

    text = text.replace(
        "/* End PBXFileReference section */",
        f"""\t\t{IDS["vpn_swift_ref"]} /* NagaVpnPlugin.swift */ = {{isa = PBXFileReference; lastKnownFileType = sourcecode.swift; path = NagaVpnPlugin.swift; sourceTree = "<group>"; }};
		{IDS["ctl_swift_ref"]} /* NagaControl.swift */ = {{isa = PBXFileReference; lastKnownFileType = sourcecode.swift; path = NagaControl.swift; sourceTree = "<group>"; }};
		{IDS["ent_runner_ref"]} /* Runner.entitlements */ = {{isa = PBXFileReference; lastKnownFileType = text.plist.entitlements; path = Runner.entitlements; sourceTree = "<group>"; }};
		{IDS["privacy_ref"]} /* PrivacyInfo.xcprivacy */ = {{isa = PBXFileReference; lastKnownFileType = text.xml; path = PrivacyInfo.xcprivacy; sourceTree = "<group>"; }};
		{IDS["pt_swift_ref"]} /* PacketTunnelProvider.swift */ = {{isa = PBXFileReference; lastKnownFileType = sourcecode.swift; path = PacketTunnelProvider.swift; sourceTree = "<group>"; }};
		{IDS["pt_plist_ref"]} /* Info.plist */ = {{isa = PBXFileReference; lastKnownFileType = text.plist.xml; path = Info.plist; sourceTree = "<group>"; }};
		{IDS["pt_ent_ref"]} /* PacketTunnel.entitlements */ = {{isa = PBXFileReference; lastKnownFileType = text.plist.entitlements; path = PacketTunnel.entitlements; sourceTree = "<group>"; }};
		{IDS["pt_product"]} /* PacketTunnel.appex */ = {{isa = PBXFileReference; explicitFileType = "wrapper.app-extension"; includeInIndex = 0; path = PacketTunnel.appex; sourceTree = BUILT_PRODUCTS_DIR; }};
/* End PBXFileReference section */""",
    )

    text = text.replace(
        f"""\t\t{IDS["pt_product"]} /* PacketTunnel.appex */ = """,
        f"""\t\t{IDS["pt_frameworks"]} /* Frameworks */ = {{
			isa = PBXFrameworksBuildPhase;
			buildActionMask = 2147483647;
			files = (
			);
			runOnlyForDeploymentPostprocessing = 0;
		}};
		"""
        + f"""\t\t{IDS["pt_product"]} /* PacketTunnel.appex */ = """,
    )
    # Frameworks phase must be in PBXFrameworksBuildPhase, not FileReference.
    # Revert accidental insert and do it properly.
    text = text.replace(
        f"""\t\t{IDS["pt_frameworks"]} /* Frameworks */ = {{
			isa = PBXFrameworksBuildPhase;
			buildActionMask = 2147483647;
			files = (
			);
			runOnlyForDeploymentPostprocessing = 0;
		}};
		{IDS["pt_product"]} /* PacketTunnel.appex */ = {{isa = PBXFileReference; explicitFileType = "wrapper.app-extension"; includeInIndex = 0; path = PacketTunnel.appex; sourceTree = BUILT_PRODUCTS_DIR; }};
""",
        f"""\t\t{IDS["pt_product"]} /* PacketTunnel.appex */ = {{isa = PBXFileReference; explicitFileType = "wrapper.app-extension"; includeInIndex = 0; path = PacketTunnel.appex; sourceTree = BUILT_PRODUCTS_DIR; }};
""",
    )

    text = text.replace(
        "/* End PBXFrameworksBuildPhase section */",
        f"""\t\t{IDS["pt_frameworks"]} /* Frameworks */ = {{
			isa = PBXFrameworksBuildPhase;
			buildActionMask = 2147483647;
			files = (
			);
			runOnlyForDeploymentPostprocessing = 0;
		}};
/* End PBXFrameworksBuildPhase section */""",
    )

    text = text.replace(
        "\t\t\t\t74858FAD1ED2DC5600515810 /* Runner-Bridging-Header.h */,\n",
        f"""\t\t\t\t74858FAD1ED2DC5600515810 /* Runner-Bridging-Header.h */,
				{IDS["vpn_swift_ref"]} /* NagaVpnPlugin.swift */,
				{IDS["ctl_swift_ref"]} /* NagaControl.swift */,
				{IDS["ent_runner_ref"]} /* Runner.entitlements */,
				{IDS["privacy_ref"]} /* PrivacyInfo.xcprivacy */,
""",
    )

    text = text.replace(
        "\t\t\t\t331C8081294A63A400263BE5 /* RunnerTests.xctest */,\n",
        f"""\t\t\t\t331C8081294A63A400263BE5 /* RunnerTests.xctest */,
				{IDS["pt_product"]} /* PacketTunnel.appex */,
""",
    )

    text = text.replace(
        "\t\t\t\t331C8082294A63A400263BE5 /* RunnerTests */,\n",
        f"""\t\t\t\t331C8082294A63A400263BE5 /* RunnerTests */,
				{IDS["pt_group"]} /* PacketTunnel */,
""",
    )

    text = text.replace(
        "/* End PBXGroup section */",
        f"""\t\t{IDS["pt_group"]} /* PacketTunnel */ = {{
			isa = PBXGroup;
			children = (
				{IDS["pt_swift_ref"]} /* PacketTunnelProvider.swift */,
				{IDS["pt_plist_ref"]} /* Info.plist */,
				{IDS["pt_ent_ref"]} /* PacketTunnel.entitlements */,
			);
			path = PacketTunnel;
			sourceTree = "<group>";
		}};
/* End PBXGroup section */""",
    )

    text = text.replace(
        "/* End PBXNativeTarget section */",
        f"""\t\t{IDS["pt_target"]} /* PacketTunnel */ = {{
			isa = PBXNativeTarget;
			buildConfigurationList = {IDS["pt_configs"]} /* Build configuration list for PBXNativeTarget "PacketTunnel" */;
			buildPhases = (
				{IDS["pt_sources"]} /* Sources */,
				{IDS["pt_frameworks"]} /* Frameworks */,
				{IDS["pt_resources"]} /* Resources */,
			);
			buildRules = (
			);
			dependencies = (
			);
			name = PacketTunnel;
			productName = PacketTunnel;
			productReference = {IDS["pt_product"]} /* PacketTunnel.appex */;
			productType = "com.apple.product-type.app-extension";
		}};
/* End PBXNativeTarget section */""",
    )

    text = text.replace(
        """					97C146ED1CF9000F007C117D = {
						CreatedOnToolsVersion = 7.3.1;
						LastSwiftMigration = 1100;
					};""",
        f"""					97C146ED1CF9000F007C117D = {{
						CreatedOnToolsVersion = 7.3.1;
						LastSwiftMigration = 1100;
					}};
					{IDS["pt_target"]} = {{
						CreatedOnToolsVersion = 16.0;
					}};""",
    )

    text = text.replace(
        "\t\t\t\t331C8080294A63A400263BE5 /* RunnerTests */,\n",
        f"""\t\t\t\t331C8080294A63A400263BE5 /* RunnerTests */,
				{IDS["pt_target"]} /* PacketTunnel */,
""",
    )

    text = text.replace(
        "\t\t\t\t3B06AD1E1E4923F5004D2608 /* Thin Binary */,\n",
        f"""\t\t\t\t3B06AD1E1E4923F5004D2608 /* Thin Binary */,
				{IDS["embed_ext"]} /* Embed App Extensions */,
""",
    )

    text = text.replace(
        """			dependencies = (
			);
			name = Runner;""",
        f"""			dependencies = (
				{IDS["pt_dep"]} /* PBXTargetDependency */,
			);
			name = Runner;""",
    )

    text = text.replace(
        "\t\t\t\t3B3967161E833CAA004F5970 /* AppFrameworkInfo.plist in Resources */,\n",
        f"""\t\t\t\t3B3967161E833CAA004F5970 /* AppFrameworkInfo.plist in Resources */,
				{IDS["privacy_build"]} /* PrivacyInfo.xcprivacy in Resources */,
""",
    )

    text = text.replace(
        "/* End PBXResourcesBuildPhase section */",
        f"""\t\t{IDS["pt_resources"]} /* Resources */ = {{
			isa = PBXResourcesBuildPhase;
			buildActionMask = 2147483647;
			files = (
			);
			runOnlyForDeploymentPostprocessing = 0;
		}};
/* End PBXResourcesBuildPhase section */""",
    )

    text = text.replace(
        "\t\t\t\t7884E8682EC3CC0700C636F2 /* SceneDelegate.swift in Sources */,\n",
        f"""\t\t\t\t7884E8682EC3CC0700C636F2 /* SceneDelegate.swift in Sources */,
				{IDS["vpn_swift_build"]} /* NagaVpnPlugin.swift in Sources */,
				{IDS["ctl_swift_build"]} /* NagaControl.swift in Sources */,
""",
    )

    text = text.replace(
        "/* End PBXSourcesBuildPhase section */",
        f"""\t\t{IDS["pt_sources"]} /* Sources */ = {{
			isa = PBXSourcesBuildPhase;
			buildActionMask = 2147483647;
			files = (
				{IDS["pt_swift_build"]} /* PacketTunnelProvider.swift in Sources */,
			);
			runOnlyForDeploymentPostprocessing = 0;
		}};
/* End PBXSourcesBuildPhase section */""",
    )

    text = text.replace(
        "/* End PBXTargetDependency section */",
        f"""\t\t{IDS["pt_dep"]} /* PBXTargetDependency */ = {{
			isa = PBXTargetDependency;
			target = {IDS["pt_target"]} /* PacketTunnel */;
			targetProxy = {IDS["pt_proxy"]} /* PBXContainerItemProxy */;
		}};
/* End PBXTargetDependency section */""",
    )

    text = text.replace("IPHONEOS_DEPLOYMENT_TARGET = 15.0;", "IPHONEOS_DEPLOYMENT_TARGET = 16.0;")
    text = text.replace(
        "PRODUCT_BUNDLE_IDENTIFIER = eu.nagavpn.nagaNetwork;",
        "PRODUCT_BUNDLE_IDENTIFIER = eu.nagavpn.naga_network;",
    )
    text = text.replace(
        "PRODUCT_BUNDLE_IDENTIFIER = eu.nagavpn.nagaNetwork.RunnerTests;",
        "PRODUCT_BUNDLE_IDENTIFIER = eu.nagavpn.naga_network.RunnerTests;",
    )

    for key in (
        "97C147061CF9000F007C117D /* Debug */",
        "97C147071CF9000F007C117D /* Release */",
        "249021D4217E4FDB00AE95B9 /* Profile */",
    ):
        needle = f"{key} = {{\n\t\t\tisa = XCBuildConfiguration;\n\t\t\tbaseConfigurationReference"
        if "CODE_SIGN_ENTITLEMENTS = Runner/Runner.entitlements;" not in text.split(key, 1)[-1][:800]:
            text = text.replace(
                f"{key} = {{\n\t\t\tisa = XCBuildConfiguration;\n\t\t\tbaseConfigurationReference",
                f"{key} = {{\n\t\t\tisa = XCBuildConfiguration;\n\t\t\tbaseConfigurationReference",
                1,
            )

    text = text.replace(
        "ENABLE_BITCODE = NO;\n\t\t\t\tINFOPLIST_FILE = Runner/Info.plist;",
        "CODE_SIGN_ENTITLEMENTS = Runner/Runner.entitlements;\n\t\t\t\tENABLE_BITCODE = NO;\n\t\t\t\tINFOPLIST_FILE = Runner/Info.plist;",
    )

    pt_settings = """
				CODE_SIGN_ENTITLEMENTS = PacketTunnel/PacketTunnel.entitlements;
				CODE_SIGN_STYLE = Automatic;
				CURRENT_PROJECT_VERSION = "$(FLUTTER_BUILD_NUMBER)";
				GENERATE_INFOPLIST_FILE = NO;
				INFOPLIST_FILE = PacketTunnel/Info.plist;
				IPHONEOS_DEPLOYMENT_TARGET = 16.0;
				LD_RUNPATH_SEARCH_PATHS = (
					"$(inherited)",
					"@executable_path/Frameworks",
					"@executable_path/../../Frameworks",
				);
				MARKETING_VERSION = "$(FLUTTER_BUILD_NAME)";
				PRODUCT_BUNDLE_IDENTIFIER = eu.nagavpn.naga_network.PacketTunnel;
				PRODUCT_NAME = PacketTunnel;
				SKIP_INSTALL = YES;
				SWIFT_EMIT_LOC_STRINGS = YES;
				SWIFT_VERSION = 5.0;
				TARGETED_DEVICE_FAMILY = "1,2";
"""

    text = text.replace(
        "/* End XCBuildConfiguration section */",
        f"""\t\t{IDS["pt_debug"]} /* Debug */ = {{
			isa = XCBuildConfiguration;
			buildSettings = {{{pt_settings}\t\t\t\tSWIFT_ACTIVE_COMPILATION_CONDITIONS = DEBUG;
				SWIFT_OPTIMIZATION_LEVEL = "-Onone";
			}};
			name = Debug;
		}};
		{IDS["pt_release"]} /* Release */ = {{
			isa = XCBuildConfiguration;
			buildSettings = {{{pt_settings}\t\t\t}};
			name = Release;
		}};
		{IDS["pt_profile"]} /* Profile */ = {{
			isa = XCBuildConfiguration;
			buildSettings = {{{pt_settings}\t\t\t}};
			name = Profile;
		}};
/* End XCBuildConfiguration section */""",
    )

    text = text.replace(
        "/* End XCConfigurationList section */",
        f"""\t\t{IDS["pt_configs"]} /* Build configuration list for PBXNativeTarget "PacketTunnel" */ = {{
			isa = XCConfigurationList;
			buildConfigurations = (
				{IDS["pt_debug"]} /* Debug */,
				{IDS["pt_release"]} /* Release */,
				{IDS["pt_profile"]} /* Profile */,
			);
			defaultConfigurationIsVisible = 0;
			defaultConfigurationName = Release;
		}};
/* End XCConfigurationList section */""",
    )

    PBX.write_text(text)
    print("patched", PBX)


if __name__ == "__main__":
    main()
