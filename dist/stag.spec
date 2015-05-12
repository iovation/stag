Name:     stag
Version:  0.4.0
Release:  0
Summary:  Stag - Statistics Aggregator

Group:      iovation, inc.
License:    Proprietary
Source0:    %{name}-%{version}.tar.gz
Prefix:     /
BuildRoot:  %(mktemp -ud %{_tmppath}/%{name}-%{version}-%{release}-XXXXXX)
BuildArch:  noarch
BuildRequires: golang
Requires(post): chkconfig
Requires(preun): chkconfig
# This is for /sbin/service
Requires(preun): initscripts

%description
wtf-consumer connects to an iovation TLM queue and gathers data, caculates
averages, histogram buckets and throughput and dumps that data to Graphite.

%prep
%setup -q

%build
go build stag.go

%install
%{__mkdir} -p $RPM_BUILD_ROOT%{prefix}/etc/init.d $RPM_BUILD_ROOT%{prefix}/usr/local/stag/bin
%{__install} -p -m 755 stag $RPM_BUILD_ROOT%{prefix}/usr/local/stag/bin

%clean
rm -rf $RPM_BUILD_ROOT

%files
%defattr(-,root,root,-)
%doc
%{prefix}/usr/*
