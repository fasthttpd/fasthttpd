# NOTE: _unitdir is not defined in rpm of ubuntu
%define unitdir /usr/lib/systemd/system
%define contentdir /usr/share/fasthttpd

Name: fasthttpd
Version: <VERSION>
Release: 1%{?dist}
Summary: FastHttpd
URL: https://github.com/fasthttpd/fasthttpd
License: MIT
Requires(pre): /usr/sbin/useradd
Requires(post): /usr/bin/systemctl

Source0: fasthttpd
Source10: fasthttpd.service
Source11: config.yaml
Source100: index.html
Source101: favicon.ico

%description
FastHttpd is a HTTP server using valyala/fasthttp.

%pre
/usr/sbin/useradd -c "FastHttpd" \
	-s /sbin/nologin -r -d %{contentdir} fasthttpd 2> /dev/null || :

%post
/usr/bin/systemctl enable fasthttpd
/usr/bin/systemctl start fasthttpd

%preun
if [ $1 = 0 ]; then
	/usr/bin/systemctl stop fasthttpd > /dev/null 2>&1
	/usr/bin/systemctl disable fasthttpd > /dev/null 2>&1
fi

%install
mkdir -p \
    %{buildroot}/%{_sbindir} \
    %{buildroot}/%{_sysconfdir} \
    %{buildroot}/%{unitdir}

install -p -d %{buildroot}/%{_sysconfdir}/fasthttpd
install -p -d %{buildroot}/%{contentdir}/html
install -p -d %{buildroot}/var/log/fasthttpd

install -p -m 0755 %{SOURCE0} %{buildroot}/%{_sbindir}
install -p -m 0644 %{SOURCE10} %{buildroot}/%{unitdir}
install -p -m 0644 %{SOURCE11} %{buildroot}/%{_sysconfdir}/fasthttpd
install -p -m 0644 \
    %{SOURCE100} \
    %{SOURCE101} \
    %{buildroot}/%{contentdir}/html

%files
%{_sbindir}/fasthttpd
%{unitdir}/fasthttpd.service
%{_sysconfdir}/fasthttpd/config.yaml
%{contentdir}/html/*