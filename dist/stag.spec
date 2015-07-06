%define debug_package %{nil}

Name:     stag
Version:  0.4.4
Release:  2
Summary:  Stag - Statistics Aggregator

Group:      iovation, inc.
License:    Proprietary
Source0:    %{name}-%{version}.tar.gz
Prefix:     /
BuildRoot:  %(mktemp -ud %{_tmppath}/%{name}-%{version}-%{release}-XXXXXX)
BuildRequires: golang
Requires(post): chkconfig
Requires(preun): chkconfig
# This is for /sbin/service
Requires(preun): initscripts

%description
Stag is a statistics aggregator that takes input similar to that of statsd and outputs it to Graphite

%prep
%setup -q
go get github.com/constabulary/gb/...

%build
cd src ; gb build all

%install
%{__mkdir} -p $RPM_BUILD_ROOT%{prefix}/etc/init.d $RPM_BUILD_ROOT%{prefix}/etc/sysconfig $RPM_BUILD_ROOT%{prefix}/usr/local/stag/bin
%{__install} -p -m 755 bin/stag $RPM_BUILD_ROOT%{prefix}/usr/local/stag/bin

%clean
rm -rf $RPM_BUILD_ROOT

%files
%defattr(-,root,root,-)
%doc
%{prefix}/usr/*
