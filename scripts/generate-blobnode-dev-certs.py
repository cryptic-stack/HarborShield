from __future__ import annotations

import ipaddress
import os
from datetime import datetime, timedelta, timezone
from pathlib import Path

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID


def write_pem(path: Path, data: bytes) -> None:
    path.write_bytes(data)


def main() -> None:
    repo_root = Path(__file__).resolve().parent.parent
    output_dir = Path(
        os.environ.get("HS_CERT_OUTPUT_DIR", str(repo_root / "deploy" / "certs" / "blobnode"))
    )
    valid_days = int(os.environ.get("HS_CERT_VALID_DAYS", "365"))
    output_dir.mkdir(parents=True, exist_ok=True)

    now = datetime.now(timezone.utc) - timedelta(minutes=5)
    expires = now + timedelta(days=valid_days)

    ca_key = rsa.generate_private_key(public_exponent=65537, key_size=4096)
    ca_subject = x509.Name(
        [x509.NameAttribute(NameOID.COMMON_NAME, "HarborShield Blobnode Dev CA")]
    )
    ca_cert = (
        x509.CertificateBuilder()
        .subject_name(ca_subject)
        .issuer_name(ca_subject)
        .public_key(ca_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now)
        .not_valid_after(expires)
        .add_extension(x509.BasicConstraints(ca=True, path_length=None), critical=True)
        .add_extension(
            x509.KeyUsage(
                digital_signature=False,
                content_commitment=False,
                key_encipherment=False,
                data_encipherment=False,
                key_agreement=False,
                key_cert_sign=True,
                crl_sign=True,
                encipher_only=False,
                decipher_only=False,
            ),
            critical=True,
        )
        .sign(ca_key, hashes.SHA256())
    )

    server_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    server_subject = x509.Name(
        [x509.NameAttribute(NameOID.COMMON_NAME, "HarborShield Blobnode Server")]
    )
    server_sans = [
        x509.DNSName("blobnode-a"),
        x509.DNSName("blobnode-b"),
        x509.DNSName("blobnode-c"),
        x509.DNSName("blobnode-d"),
        x509.DNSName("localhost"),
        x509.IPAddress(ipaddress.ip_address("127.0.0.1")),
    ]
    server_cert = (
        x509.CertificateBuilder()
        .subject_name(server_subject)
        .issuer_name(ca_cert.subject)
        .public_key(server_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now)
        .not_valid_after(expires)
        .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
        .add_extension(x509.SubjectAlternativeName(server_sans), critical=False)
        .add_extension(
            x509.ExtendedKeyUsage([ExtendedKeyUsageOID.SERVER_AUTH]), critical=False
        )
        .add_extension(
            x509.KeyUsage(
                digital_signature=True,
                content_commitment=False,
                key_encipherment=True,
                data_encipherment=False,
                key_agreement=False,
                key_cert_sign=False,
                crl_sign=False,
                encipher_only=False,
                decipher_only=False,
            ),
            critical=True,
        )
        .sign(ca_key, hashes.SHA256())
    )

    client_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    client_subject = x509.Name(
        [x509.NameAttribute(NameOID.COMMON_NAME, "HarborShield Blobnode Client")]
    )
    client_cert = (
        x509.CertificateBuilder()
        .subject_name(client_subject)
        .issuer_name(ca_cert.subject)
        .public_key(client_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now)
        .not_valid_after(expires)
        .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
        .add_extension(
            x509.ExtendedKeyUsage([ExtendedKeyUsageOID.CLIENT_AUTH]), critical=False
        )
        .add_extension(
            x509.KeyUsage(
                digital_signature=True,
                content_commitment=False,
                key_encipherment=False,
                data_encipherment=False,
                key_agreement=False,
                key_cert_sign=False,
                crl_sign=False,
                encipher_only=False,
                decipher_only=False,
            ),
            critical=True,
        )
        .sign(ca_key, hashes.SHA256())
    )

    write_pem(
        output_dir / "ca.crt",
        ca_cert.public_bytes(serialization.Encoding.PEM),
    )
    write_pem(
        output_dir / "server.crt",
        server_cert.public_bytes(serialization.Encoding.PEM),
    )
    write_pem(
        output_dir / "server.key",
        server_key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.PKCS8,
            encryption_algorithm=serialization.NoEncryption(),
        ),
    )
    write_pem(
        output_dir / "client.crt",
        client_cert.public_bytes(serialization.Encoding.PEM),
    )
    write_pem(
        output_dir / "client.key",
        client_key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.PKCS8,
            encryption_algorithm=serialization.NoEncryption(),
        ),
    )

    print(f"Generated blobnode dev certificates in {output_dir}")
    print("- ca.crt")
    print("- server.crt")
    print("- server.key")
    print("- client.crt")
    print("- client.key")


if __name__ == "__main__":
    main()
