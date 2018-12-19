FROM mysql:5.7.22
ADD https://repo.percona.com/apt/percona-release_0.1-7.stretch_all.deb /
RUN dpkg -i /percona-release_0.1-7.stretch_all.deb && apt-get update
RUN apt-get install -y pmm-client
RUN apt-get install -y procps
RUN apt-get install -y iproute